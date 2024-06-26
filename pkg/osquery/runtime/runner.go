package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"

	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquerylogger "github.com/osquery/osquery-go/plugin/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// How long to wait before erroring because we cannot open the osquery
	// extension socket.
	socketOpenTimeout = 10 * time.Second

	// How often to try to open the osquery extension socket
	socketOpenInterval = 200 * time.Millisecond

	// How frequently we should healthcheck the client/server
	healthCheckInterval = 60 * time.Second

	// The maximum amount of time to wait for the osquery socket to be available -- overrides context deadline
	maxSocketWaitTime = 30 * time.Second
)

type Runner struct {
	instance     *OsqueryInstance
	instanceLock sync.Mutex
	slogger      *slog.Logger
	knapsack     types.Knapsack
	shutdown     chan struct{}
	interrupted  bool
	opts         []OsqueryInstanceOption
}

func New(k types.Knapsack, opts ...OsqueryInstanceOption) *Runner {
	runner := newRunner(opts...)
	runner.slogger = k.Slogger().With("component", "osquery_runner")
	runner.knapsack = k

	k.RegisterChangeObserver(runner,
		keys.WatchdogEnabled, keys.WatchdogMemoryLimitMB, keys.WatchdogUtilizationLimitPercent, keys.WatchdogDelaySec,
	)

	return runner
}

func (r *Runner) Run() error {
	if err := r.launchOsqueryInstance(); err != nil {
		return fmt.Errorf("starting instance: %w", err)
	}

	// This loop waits for the completion of the async routines,
	// and either restarts the instance (if Shutdown was not
	// called), or stops (if Shutdown was called).
	for {
		// Wait for async processes to exit
		<-r.instance.doneCtx.Done()
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"osquery instance exited",
		)

		select {
		case <-r.shutdown:
			// Intentional shutdown, this loop can exit
			if err := r.instance.stats.Exited(nil); err != nil {
				r.slogger.Log(context.TODO(), slog.LevelWarn,
					"error recording osquery instance exit to history",
					"err", err,
				)
			}
			return nil
		default:
			// Don't block
		}

		// Error case -- osquery instance shut down and needs to be restarted
		err := r.instance.errgroup.Wait()
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"unexpected restart of instance",
			"err", err,
		)

		if err := r.instance.stats.Exited(err); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelWarn,
				"error recording osquery instance exit to history",
				"err", err,
			)
		}

		r.instanceLock.Lock()
		opts := r.instance.opts
		r.instance = newInstance()
		r.instance.opts = opts
		for _, opt := range r.opts {
			opt(r.instance)
		}
		if err := r.launchOsqueryInstance(); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelWarn,
				"fatal error restarting instance, shutting down",
				"err", err,
			)
			r.instanceLock.Unlock()
			if err := r.Shutdown(); err != nil {
				r.slogger.Log(context.TODO(), slog.LevelWarn,
					"could not perform shutdown",
					"err", err,
				)
			}

			// Failed to restart instance -- exit rungroup so launcher can reload
			return fmt.Errorf("restarting instance after unexpected exit: %w", err)
		}

		r.instanceLock.Unlock()
	}
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Query(query)
}

func (r *Runner) Interrupt(_ error) {
	if err := r.Shutdown(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not shut down runner on interrupt",
			"err", err,
		)
	}
}

// Shutdown instructs the runner to permanently stop the running instance (no
// restart will be attempted).
func (r *Runner) Shutdown() error {
	if r.interrupted {
		// Already shut down, nothing else to do
		return nil
	}

	r.interrupted = true
	close(r.shutdown)
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	r.instance.cancel()
	if err := r.instance.errgroup.Wait(); err != context.Canceled && err != nil {
		return fmt.Errorf("while shutting down instance: %w", err)
	}
	return nil
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which are enable_watchdog, watchdog_delay_sec, watchdog_memory_limit_mb,
// and watchdog_utilization_limit_percent.
func (r *Runner) FlagsChanged(flagKeys ...keys.FlagKey) {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"control server flags changed, restarting instance to apply",
		"flags", fmt.Sprintf("%+v", flagKeys),
	)

	if err := r.Restart(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not restart osquery instance after flag change",
			"err", err,
		)
	}
}

// Ping satisfies the control.subscriber interface -- the runner subscribes to changes to
// the katc_config subsystem.
func (r *Runner) Ping() {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"Kolide ATC configuration changed, restarting instance to apply",
	)

	if err := r.Restart(); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not restart osquery instance after Kolide ATC configuration changed",
			"err", err,
		)
	}
}

// Restart allows you to cleanly shutdown the current instance and launch a new
// instance with the same configurations.
func (r *Runner) Restart() error {
	r.slogger.Log(context.TODO(), slog.LevelDebug,
		"runner.Restart called",
	)
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
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	o := r.instance

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if o.opts.rootDirectory == "" {
		rootDirectory, rmRootDirectory, err := osqueryTempDir()
		if err != nil {
			traces.SetError(span, fmt.Errorf("couldn't create temp directory for osquery instance: %w", err))
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
		traces.SetError(span, fmt.Errorf("could not calculate osquery file paths: %w", err))
		return fmt.Errorf("could not calculate osquery file paths: %w", err)
	}

	// Populate augeas lenses, if requested
	if o.opts.augeasLensFunc != nil {
		if err := os.MkdirAll(paths.augeasPath, 0755); err != nil {
			traces.SetError(span, fmt.Errorf("making augeas lenses directory: %w", err))
			return fmt.Errorf("making augeas lenses directory: %w", err)
		}

		if err := o.opts.augeasLensFunc(paths.augeasPath); err != nil {
			traces.SetError(span, fmt.Errorf("setting up augeas lenses: %w", err))
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
		logString := func(ctx context.Context, typ osquerylogger.LogType, logText string) error {
			return nil
		}
		o.opts.extensionPlugins = append(o.opts.extensionPlugins, osquerylogger.NewPlugin("internal_noop", logString))
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

	// The knapsack will retrieve the correct version of osqueryd from the download library if available.
	// If not available, it will fall back to the configured installed version of osqueryd.
	currentOsquerydBinaryPath := o.knapsack.LatestOsquerydPath(ctx)
	span.AddEvent("got_osqueryd_binary_path", trace.WithAttributes(attribute.String("path", currentOsquerydBinaryPath)))

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	o.cmd, err = o.createOsquerydCommand(currentOsquerydBinaryPath, paths)
	if err != nil {
		traces.SetError(span, fmt.Errorf("couldn't create osqueryd command: %w", err))
		return fmt.Errorf("couldn't create osqueryd command: %w", err)
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	o.cmd.SysProcAttr = setpgid()

	r.slogger.Log(ctx, slog.LevelInfo,
		"launching osqueryd",
		"path", o.cmd.Path,
		"args", strings.Join(o.cmd.Args, " "),
	)

	// remove any socket already at the extension socket path to ensure
	// that it's not left over from a previous instance
	if err := os.RemoveAll(paths.extensionSocketPath); err != nil {
		r.slogger.Log(ctx, slog.LevelWarn,
			"error removing osquery extension socket",
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
			"err", err,
		)

		r.slogger.Log(ctx, slog.LevelWarn,
			"fatal error starting osquery -- could not exec.",
			msgPairs...,
		)
		traces.SetError(span, fmt.Errorf("fatal error starting osqueryd process: %w", err))
		return fmt.Errorf("fatal error starting osqueryd process: %w", err)
	}

	span.AddEvent("launched_osqueryd")
	r.slogger.Log(ctx, slog.LevelInfo,
		"launched osquery process",
		"osqueryd_pid", o.cmd.Process.Pid,
	)

	// wait for osquery to create the socket before moving on,
	// this is intended to serve as a kind of health check
	// for osquery, if it's started successfully it will create
	// a socket
	if err := backoff.WaitFor(func() error {
		_, err := os.Stat(paths.extensionSocketPath)
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"osquery extension socket not created yet ... will retry",
				"path", paths.extensionSocketPath,
			)
		}
		return err
	}, 1*time.Minute, 1*time.Second); err != nil {
		traces.SetError(span, fmt.Errorf("timeout waiting for osqueryd to create socket at %s: %w", paths.extensionSocketPath, err))
		return fmt.Errorf("timeout waiting for osqueryd to create socket at %s: %w", paths.extensionSocketPath, err)
	}

	span.AddEvent("socket_created")

	stats, err := history.NewInstance()
	if err != nil {
		r.slogger.Log(ctx, slog.LevelWarn,
			"could not create new osquery instance history",
			"err", err,
		)
	}
	o.stats = stats

	// This loop runs in the background when the process was
	// successfully started. ("successful" is independent of exit
	// code. eg: this runs if we could exec. Failure to exec is above.)
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "monitor osquery process",
		)

		err := o.cmd.Wait()
		switch {
		case err == nil, isExitOk(err):
			r.slogger.Log(ctx, slog.LevelInfo,
				"osquery exited successfully",
			)
			// TODO: should this return nil?
			return errors.New("osquery process exited successfully")
		default:
			msgPairs := append(
				getOsqueryInfoForLog(o.cmd.Path),
				"err", err,
			)

			r.slogger.Log(ctx, slog.LevelWarn,
				"error running osquery command",
				msgPairs...,
			)
			return fmt.Errorf("running osqueryd command: %w", err)
		}
	})

	// Kill osquery process on shutdown
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "kill osquery process on shutdown",
		)

		<-o.doneCtx.Done()
		r.slogger.Log(ctx, slog.LevelDebug,
			"starting osquery shutdown",
		)
		if o.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(o.cmd); err != nil {
				if strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
					r.slogger.Log(ctx, slog.LevelDebug,
						"tried to stop osquery, but process already gone",
					)
				} else {
					r.slogger.Log(ctx, slog.LevelWarn,
						"error killing osquery process",
						"err", err,
					)
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
	o.extensionManagerClient, err = o.StartOsqueryClient(paths)
	if err != nil {
		traces.SetError(span, fmt.Errorf("could not create an extension client: %w", err))
		return fmt.Errorf("could not create an extension client: %w", err)
	}
	span.AddEvent("extension_client_created")

	if len(o.opts.extensionPlugins) > 0 {
		if err := o.StartOsqueryExtensionManagerServer("kolide_grpc", paths.extensionSocketPath, o.extensionManagerClient, o.opts.extensionPlugins); err != nil {
			r.slogger.Log(ctx, slog.LevelInfo,
				"unable to create initial extension server, stopping",
				"err", err,
			)
			traces.SetError(span, fmt.Errorf("could not create an extension server: %w", err))
			return fmt.Errorf("could not create an extension server: %w", err)
		}
		span.AddEvent("extension_server_created")
	}

	if err := o.stats.Connected(o); err != nil {
		r.slogger.Log(ctx, slog.LevelWarn,
			"could not set connection time for osquery instance history",
			"err", err,
		)
	}

	// Now spawn an extension manager for the tables. We need to
	// start this one in the background, because the runner.Start
	// function needs to return promptly enough for osquery to use
	// it to enroll. Very racy
	//
	// TODO: Consider chunking, if we find we can only have so
	// many tables per extension manager
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "kolide extension manager server launch",
		)

		plugins := table.PlatformTables(r.knapsack, r.knapsack.Slogger().With("component", "platform_tables"), currentOsquerydBinaryPath)

		if len(plugins) == 0 {
			return nil
		}

		if err := o.StartOsqueryExtensionManagerServer("kolide", paths.extensionSocketPath, o.extensionManagerClient, plugins); err != nil {
			r.slogger.Log(ctx, slog.LevelWarn,
				"unable to create tables extension server, stopping",
				"err", err,
			)
			return fmt.Errorf("could not create a table extension server: %w", err)
		}
		return nil
	})

	// Health check on interval
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "health check on interval",
		)

		if o.knapsack != nil && o.knapsack.OsqueryHealthcheckStartupDelay() != 0*time.Second {
			r.slogger.Log(ctx, slog.LevelDebug,
				"entering delay before starting osquery healthchecks",
			)
			select {
			case <-time.After(o.knapsack.OsqueryHealthcheckStartupDelay()):
				r.slogger.Log(ctx, slog.LevelDebug,
					"exiting delay before starting osquery healthchecks",
				)
			case <-o.doneCtx.Done():
				return o.doneCtx.Err()
			}
		}

		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-o.doneCtx.Done():
				return o.doneCtx.Err()
			case <-ticker.C:
				// If device is sleeping, we do not want to perform unnecessary healthchecks that
				// may force an unnecessary restart.
				if o.knapsack != nil && o.knapsack.InModernStandby() {
					break
				}

				// Health check! Allow a couple
				// failures before we tear everything
				// down. This is pretty simple, it
				// hardcodes the timing. Might be
				// better for a Limiter
				maxHealthChecks := 5
				for i := 1; i <= maxHealthChecks; i++ {
					err := r.Healthy()
					if err == nil {
						// err was nil, clear failed attempts
						if i > 1 {
							r.slogger.Log(ctx, slog.LevelDebug,
								"healthcheck passed, clearing error",
								"attempt", i,
							)
						}
						break
					}

					if i == maxHealthChecks {
						r.slogger.Log(ctx, slog.LevelInfo,
							"healthcheck failed, giving up",
							"attempt", i,
							"err", err,
						)
						return fmt.Errorf("health check failed: %w", err)
					}

					r.slogger.Log(ctx, slog.LevelDebug,
						"healthcheck failed, will retry",
						"attempt", i,
						"err", err,
					)
					time.Sleep(1 * time.Second)
				}
			}
		}
	})

	// Cleanup temp dir
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "cleanup temp dir",
		)

		<-o.doneCtx.Done()
		if o.usingTempDir && o.rmRootDirectory != nil {
			o.rmRootDirectory()
		}
		return o.doneCtx.Err()
	})

	// Clean up PID file on shutdown
	o.errgroup.Go(func() error {
		defer r.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "cleanup PID file",
		)

		<-o.doneCtx.Done()
		if err := os.Remove(paths.pidfilePath); err != nil {
			r.slogger.Log(ctx, slog.LevelInfo,
				"could not remove PID file",
				"pid_file", paths.pidfilePath,
				"err", err,
			)
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
		opts:     opts,
	}
}
