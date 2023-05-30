package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
)

// How long to wait before erroring because we cannot open the osquery
// extension socket.
const socketOpenTimeout = 10 * time.Second

// How often to try to open the osquery extension socket
const socketOpenInterval = 200 * time.Millisecond

const healthCheckInterval = 60 * time.Second

type Runner struct {
	instance     *OsqueryInstance
	instanceLock sync.Mutex
	shutdown     chan struct{}
}

// LaunchInstance will launch an instance of osqueryd via a very configurable
// API as defined by the various OsqueryInstanceOption functional options. The
// returned instance should be shut down via the Shutdown() method.
// For example, a more customized caller might do something like the following:
//
//	  instance, err := LaunchInstance(
//	    WithOsquerydBinary("/usr/local/bin/osqueryd"),
//	    WithRootDirectory("/var/foobar"),
//	    WithConfigPluginFlag("custom"),
//			 WithOsqueryExtensionPlugins(
//			 	 config.NewPlugin("custom", custom.GenerateConfigs),
//			   logger.NewPlugin("custom", custom.LogString),
//			 	 tables.NewPlugin("foobar", custom.FoobarColumns, custom.FoobarGenerate),
//	    ),
//	  )
func LaunchInstance(opts ...OsqueryInstanceOption) (*Runner, error) {
	runner := newRunner(opts...)
	if err := runner.Start(); err != nil {
		return nil, err
	}
	return runner, nil
}

// LaunchUnstartedInstance sets up a osqueryd instance similar to LaunchInstance, but gives the caller control over
// when the instance will run. Useful for controlling startup and shutdown goroutines.
func LaunchUnstartedInstance(opts ...OsqueryInstanceOption) *Runner {
	runner := newRunner(opts...)
	return runner
}

func (r *Runner) Start() error {
	if err := r.launchOsqueryInstance(); err != nil {
		return fmt.Errorf("starting instance: %w", err)
	}
	go func() {
		// This loop waits for the completion of the async routines,
		// and either restarts the instance (if Shutdown was not
		// called), or stops (if Shutdown was called).
		for {
			// Wait for async processes to exit
			<-r.instance.doneCtx.Done()

			select {
			case <-r.shutdown:
				// Intentional shutdown, this loop can exit
				if err := r.instance.stats.Exited(nil); err != nil {
					level.Info(r.instance.logger).Log("msg", "error recording osquery instance exit to history", "err", err)
				}
				return
			default:
				// Don't block
			}

			// Error case
			err := r.instance.errgroup.Wait()
			level.Info(r.instance.logger).Log(
				"msg", "unexpected restart of instance",
				"err", err,
			)

			if err := r.instance.stats.Exited(err); err != nil {
				level.Info(r.instance.logger).Log("msg", "error recording osquery instance exit to history", "err", err)
			}

			r.instanceLock.Lock()
			opts := r.instance.opts
			r.instance = newInstance()
			r.instance.opts = opts
			if err := r.launchOsqueryInstance(); err != nil {
				level.Info(r.instance.logger).Log(
					"msg", "fatal error restarting instance",
					"err", err,
				)
				os.Exit(1)
			}

			r.instanceLock.Unlock()

		}
	}()
	return nil
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Query(query)
}

// Shutdown instructs the runner to permanently stop the running instance (no
// restart will be attempted).
func (r *Runner) Shutdown() error {
	close(r.shutdown)
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	r.instance.cancel()
	if err := r.instance.errgroup.Wait(); err != context.Canceled && err != nil {
		return fmt.Errorf("while shutting down instance: %w", err)
	}
	return nil
}

// Restart allows you to cleanly shutdown the current instance and launch a new
// instance with the same configurations.
func (r *Runner) Restart() error {
	level.Debug(r.instance.logger).Log("msg", "runner.Restart called")
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	// Cancelling will cause all of the cleanup routines to execute, and a
	// new instance will start.
	r.instance.cancel()
	r.instance.errgroup.Wait()

	return nil
}

// Healthy checks the health of the instance and returns an error describing
// any problem.
func (r *Runner) Healthy() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Healthy()
}

func (r *Runner) launchOsqueryInstance() error {
	o := r.instance

	// What binary name to look for
	lookFor := "osqueryd"
	if runtime.GOOS == "windows" {
		lookFor = lookFor + ".exe"
	}

	// If the path of the osqueryd binary wasn't explicitly defined by the caller,
	// try to find it in the path.
	if o.opts.binaryPath == "" {
		path, err := exec.LookPath(lookFor)
		if err != nil {
			return fmt.Errorf("osqueryd not supplied and not found: %w", err)
		}
		o.opts.binaryPath = path
	}

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if o.opts.rootDirectory == "" {
		rootDirectory, rmRootDirectory, err := osqueryTempDir()
		if err != nil {
			return fmt.Errorf("couldn't create temp directory for osquery instance: %w", err)
		}
		o.opts.rootDirectory = rootDirectory
		o.rmRootDirectory = rmRootDirectory
		o.usingTempDir = true
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(o.opts)
	if err != nil {
		return fmt.Errorf("could not calculate osquery file paths: %w", err)
	}

	for _, path := range paths.extensionPaths {
		// The extensions files should be owned by the process's UID or by root.
		// Osquery will refuse to load the extension otherwise.
		if err := ensureProperPermissions(o, path); err != nil {
			level.Info(o.logger).Log(
				"msg", "unable to ensure proper permissions on extension path",
				"path", path,
				"err", err,
			)
		}
	}

	// Populate augeas lenses, if requested
	if o.opts.augeasLensFunc != nil {
		if err := os.MkdirAll(paths.augeasPath, 0755); err != nil {
			return fmt.Errorf("making augeas lenses directory: %w", err)
		}

		if err := o.opts.augeasLensFunc(paths.augeasPath); err != nil {
			return fmt.Errorf("setting up augeas lenses: %w", err)
		}
	}

	// If a config plugin has not been set by the caller, then it is likely
	// that the instance will just be used for executing queries, so we
	// will use a minimal config plugin that basically is a no-op.
	if o.opts.configPluginFlag == "" {
		generateConfigs := func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		}
		o.opts.extensionPlugins = append(o.opts.extensionPlugins, config.NewPlugin("internal_noop", generateConfigs))
		o.opts.configPluginFlag = "internal_noop"
	}

	// If a logger plugin has not been set by the caller, we set a logger
	// plugin that outputs logs to the default application logger.
	if o.opts.loggerPluginFlag == "" {
		logString := func(ctx context.Context, typ logger.LogType, logText string) error {
			return nil
		}
		o.opts.extensionPlugins = append(o.opts.extensionPlugins, logger.NewPlugin("internal_noop", logString))
		o.opts.loggerPluginFlag = "internal_noop"
	}

	// If a distributed plugin has not been set by the caller, we set a
	// distributed plugin that returns no queries.
	if o.opts.distributedPluginFlag == "" {
		getQueries := func(ctx context.Context) (*distributed.GetQueriesResult, error) {
			return &distributed.GetQueriesResult{}, nil
		}
		writeResults := func(ctx context.Context, results []distributed.Result) error {
			return nil
		}
		o.opts.extensionPlugins = append(o.opts.extensionPlugins, distributed.NewPlugin("internal_noop", getQueries, writeResults))
		o.opts.distributedPluginFlag = "internal_noop"
	}

	// If we're on windows, ensure that we're looking for the .exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(o.opts.binaryPath, ".exe") {
		o.opts.binaryPath = o.opts.binaryPath + ".exe"
	}

	// before we start osqueryd, check with the update system to
	// see if we have the newest version. Do this everytime. If
	// this proves undesirable, we can expose a function to set
	// o.opts.binaryPath in the finalizer to call.
	//
	// FindNewest uses context as a way to get a logger, so we need to
	// create and pass a ctxlog in.
	currentOsquerydBinaryPath := autoupdate.FindNewest(
		ctxlog.NewContext(context.TODO(), o.logger),
		o.opts.binaryPath,
		autoupdate.DeleteOldUpdates(),
	)

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	o.cmd, err = o.opts.createOsquerydCommand(currentOsquerydBinaryPath, paths)
	if err != nil {
		return fmt.Errorf("couldn't create osqueryd command: %w", err)
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	o.cmd.SysProcAttr = setpgid()

	level.Info(o.logger).Log(
		"msg", "launching osqueryd",
		"arg0", o.cmd.Path,
		"args", strings.Join(o.cmd.Args, " "),
	)

	// remove any socket already at the extension socket path to ensure
	// that it's not left over from a previous instance
	if err := os.RemoveAll(paths.extensionSocketPath); err != nil {
		level.Info(o.logger).Log(
			"msg", "error removing osquery extension socket",
			"path", paths.extensionSocketPath,
			"err", err,
		)
	}

	// Launch osquery process (async)
	err = o.startFunc(o.cmd)
	if err != nil {
		// Failure here is indicative of a failure to exec. A missing
		// binary? Bad permissions? TODO: Consider catching errors in the
		// update system and falling back to an earlier version.
		msgPairs := append(
			getOsqueryInfoForLog(o.cmd.Path),
			"msg", "Fatal error starting osquery. Could not exec.",
			"err", err,
		)

		level.Info(o.logger).Log(msgPairs...)
		return fmt.Errorf("fatal error starting osqueryd process: %w", err)
	}

	// wait for osquery to create the socket before moving on,
	// this is intended to serve as a kind of health check
	// for osquery, if it's started successfully it will create
	// a socket
	if err := backoff.WaitFor(func() error {
		_, err := os.Stat(paths.extensionSocketPath)
		if err != nil {
			level.Debug(o.logger).Log("msg", "osquery extension socket not created yet ... will retry", "path", paths.extensionSocketPath)
		}
		return err
	}, 1*time.Minute, 1*time.Second); err != nil {
		return fmt.Errorf("timeout waiting for osqueryd to create socket at %s: %w", paths.extensionSocketPath, err)
	}

	stats, err := history.NewInstance()
	if err != nil {
		level.Info(o.logger).Log("msg", fmt.Sprint("osquery instance history error: ", err.Error()))
	}
	o.stats = stats

	// This loop runs in the background when the process was
	// successfully started. ("successful" is independent of exit
	// code. eg: this runs if we could exec. Failure to exec is above.)
	o.errgroup.Go(func() error {
		err := o.cmd.Wait()
		switch {
		case err == nil, isExitOk(err):
			level.Info(o.logger).Log("msg", "osquery exited successfully")
			// TODO: should this return nil?
			return errors.New("osquery process exited successfully")
		default:
			msgPairs := append(
				getOsqueryInfoForLog(o.cmd.Path),
				"msg", "Error running osquery command",
				"err", err,
			)

			level.Info(o.logger).Log(msgPairs...)
			return fmt.Errorf("running osqueryd command: %w", err)
		}
	})

	// Kill osquery process on shutdown
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		level.Debug(o.logger).Log("msg", "Starting osquery shutdown")
		if o.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(o.cmd); err != nil {
				if strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
					level.Debug(o.logger).Log("msg", "tried to stop osquery, but process already gone")
				} else {
					level.Info(o.logger).Log("msg", "killing osquery process", "err", err)
				}
			}
		}
		return o.doneCtx.Err()
	})

	// Here be dragons
	//
	// There are two thorny issues. First, we "invert" control of
	// the osquery process. We don't really know when osquery will
	// be running, so we need a bunch of retries on these connections
	//
	// Second, because launcher supplements the enroll
	// information, this Start function must return fast enough
	// that osquery can use the registered tables for
	// enrollment. *But* there's been a lot of racy behaviors,
	// likely due to time spent registering tables, and subtle
	// ordering issues.

	// Start an extension manager for the extensions that osquery
	// needs for config/log/etc. It's called `kolide_grpc` for mostly historic reasons
	if len(o.opts.extensionPlugins) > 0 {
		if err := o.StartOsqueryExtensionManagerServer("kolide_grpc", paths.extensionSocketPath, o.opts.extensionPlugins); err != nil {
			level.Info(o.logger).Log("msg", "Unable to create initial extension server. Stopping", "err", err)
			return fmt.Errorf("could not create an extension server: %w", err)
		}
	}

	o.extensionManagerClient, err = o.StartOsqueryClient(paths)
	if err != nil {
		return fmt.Errorf("could not create an extension client: %w", err)
	}

	if err := o.stats.Connected(o); err != nil {
		level.Info(o.logger).Log("msg", "osquery instance history", "error", err)
	}

	// Now spawn an extension manage to for the tables. We need to
	// start this one in the background, because the runner.Start
	// function needs to return promptly enough for osquery to use
	// it to enroll. Very racy
	//
	// TODO: Consider chunking, if we find we can only have so
	// many tables per extension manager
	o.errgroup.Go(func() error {
		plugins := table.PlatformTables(o.extensionManagerClient, o.logger, currentOsquerydBinaryPath)

		if len(plugins) == 0 {
			return nil
		}

		if err := o.StartOsqueryExtensionManagerServer("kolide", paths.extensionSocketPath, plugins); err != nil {
			level.Info(o.logger).Log("msg", "Unable to create tables extension server. Stopping", "err", err)
			return fmt.Errorf("could not create a table extension server: %w", err)
		}
		return nil
	})

	// Health check on interval
	o.errgroup.Go(func() error {
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-o.doneCtx.Done():
				return o.doneCtx.Err()
			case <-ticker.C:
				// Health check! Allow a couple
				// failures before we tear everything
				// down. This is pretty simple, it
				// hardcodes the timing. Might be
				// better for a Limiter
				maxHealthChecks := 5
				for i := 1; i <= maxHealthChecks; i++ {
					if err := r.Healthy(); err != nil {
						if i == maxHealthChecks {
							level.Info(o.logger).Log("msg", "Health check failed. Giving up", "attempt", i, "err", err)
							return fmt.Errorf("health check failed: %w", err)
						}

						level.Debug(o.logger).Log("msg", "Health check failed. Will retry", "attempt", i, "err", err)
						time.Sleep(1 * time.Second)

					} else {
						// err was nil, clear failed
						if i > 1 {
							level.Debug(o.logger).Log("msg", "Health check passed. Clearing error", "attempt", i)
						}

						break
					}

				}
			}
		}
	})

	// Cleanup temp dir
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		if o.usingTempDir && o.rmRootDirectory != nil {
			o.rmRootDirectory()
		}
		return o.doneCtx.Err()
	})

	return nil
}

func newRunner(opts ...OsqueryInstanceOption) *Runner {
	// Create an OsqueryInstance and apply the functional options supplied by the
	// caller.
	i := newInstance()

	for _, opt := range opts {
		opt(i)
	}

	return &Runner{
		instance: i,
		shutdown: make(chan struct{}),
	}
}
