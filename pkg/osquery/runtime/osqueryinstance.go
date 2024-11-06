package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/backoff"
	launcherosq "github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquerylogger "github.com/osquery/osquery-go/plugin/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"golang.org/x/sync/errgroup"
)

const (
	// KolideSaasExtensionName is the name of the extension that provides the config,
	// distributed queries, and log destination for the osquery process. It also provides
	// provides Kolide's additional tables: platform tables and launcher tables. This extension
	// is required for osquery startup. It is called kolide_grpc for mostly historic reasons;
	// communication with Kolide SaaS happens over JSONRPC.
	KolideSaasExtensionName = "kolide_grpc"

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

// OsqueryInstanceOption is a functional option pattern for defining how an
// osqueryd instance should be configured. For more information on this pattern,
// see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type OsqueryInstanceOption func(*OsqueryInstance)

// WithExtensionSocketPath is a functional option which allows the user to
// define the path of the extension socket path that osqueryd will open to
// communicate with other processes.
func WithExtensionSocketPath(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.extensionSocketPath = path
	}
}

// WithStdout is a functional option which allows the user to define where the
// stdout of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStdout(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.stdout = w
	}
}

// WithStderr is a functional option which allows the user to define where the
// stderr of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStderr(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.stderr = w
	}
}

// WithAugeasLensFunction defines a callback function. This can be
// used during setup to populate the augeas lenses directory.
func WithAugeasLensFunction(f func(dir string) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.augeasLensFunc = f
	}
}

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	opts           osqueryOptions
	registrationId string
	knapsack       types.Knapsack
	slogger        *slog.Logger
	serviceClient  service.KolideService
	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	runId                   string // string identifier for this instance
	errgroup                *errgroup.Group
	saasExtension           *launcherosq.Extension
	doneCtx                 context.Context // nolint:containedctx
	cancel                  context.CancelFunc
	cmd                     *exec.Cmd
	emsLock                 sync.RWMutex // Lock for extensionManagerServers
	extensionManagerServers []*osquery.ExtensionManagerServer
	extensionManagerClient  *osquery.ExtensionManagerClient
	stats                   *history.Instance
	startFunc               func(cmd *exec.Cmd) error
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy. If the instance is healthy, it returns nil.
func (i *OsqueryInstance) Healthy() error {
	// Do not add/remove servers from i.extensionManagerServers while we're accessing them
	i.emsLock.RLock()
	defer i.emsLock.RUnlock()

	if len(i.extensionManagerServers) == 0 || i.extensionManagerClient == nil {
		return errors.New("instance not started")
	}

	for _, srv := range i.extensionManagerServers {
		serverStatus, err := srv.Ping(context.TODO())
		if err != nil {
			return fmt.Errorf("could not ping extension server: %w", err)
		}
		if serverStatus.Code != 0 {
			return fmt.Errorf("ping extension server returned %d: %s",
				serverStatus.Code,
				serverStatus.Message)

		}
	}

	clientStatus, err := i.extensionManagerClient.Ping()
	if err != nil {
		return fmt.Errorf("could not ping osquery extension client: %w", err)
	}
	if clientStatus.Code != 0 {
		return fmt.Errorf("ping extension client returned %d: %s",
			clientStatus.Code,
			clientStatus.Message)

	}

	return nil
}

func (i *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	if i.extensionManagerClient == nil {
		return nil, errors.New("client not ready")
	}

	resp, err := i.extensionManagerClient.QueryContext(ctx, query)
	if err != nil {
		traces.SetError(span, err)
		return nil, fmt.Errorf("could not query the extension manager client: %w", err)
	}
	if resp.Status.Code != int32(0) {
		traces.SetError(span, errors.New(resp.Status.Message))
		return nil, errors.New(resp.Status.Message)
	}

	return resp.Response, nil
}

type osqueryOptions struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	augeasLensFunc      func(dir string) error
	extensionSocketPath string
	stderr              io.Writer
	stdout              io.Writer
}

func newInstance(registrationId string, knapsack types.Knapsack, serviceClient service.KolideService, opts ...OsqueryInstanceOption) *OsqueryInstance {
	runId := ulid.New()
	i := &OsqueryInstance{
		registrationId: registrationId,
		knapsack:       knapsack,
		slogger:        knapsack.Slogger().With("component", "osquery_instance", "instance_run_id", runId),
		serviceClient:  serviceClient,
		runId:          runId,
	}

	for _, opt := range opts {
		opt(i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel
	i.errgroup, i.doneCtx = errgroup.WithContext(ctx)

	i.startFunc = func(cmd *exec.Cmd) error {
		return cmd.Start()
	}

	return i
}

// BeginShutdown cancels the context associated with the errgroup.
func (i *OsqueryInstance) BeginShutdown() {
	i.slogger.Log(context.TODO(), slog.LevelInfo,
		"instance shutdown requested",
	)
	i.cancel()
}

// WaitShutdown waits for the instance's errgroup routines to exit, then returns the
// initial error. It should be called after either `Exited` has returned, or after
// the instance has been asked to shut down via call to `BeginShutdown`.
func (i *OsqueryInstance) WaitShutdown() error {
	// Wait for shutdown to complete
	exitErr := i.errgroup.Wait()

	// Record shutdown in stats, if initialized
	if i.stats != nil {
		if err := i.stats.Exited(exitErr); err != nil {
			i.slogger.Log(context.TODO(), slog.LevelWarn,
				"error recording osquery instance exit to history",
				"exit_err", exitErr,
				"err", err,
			)
		}
	}

	return exitErr
}

// Exited returns a channel to monitor for signal that instance has shut itself down
func (i *OsqueryInstance) Exited() <-chan struct{} {
	return i.doneCtx.Done()
}

// Launch starts the osquery instance and its components. It will run until one of its
// components becomes unhealthy, or until it is asked to shutdown via `BeginShutdown`.
func (i *OsqueryInstance) Launch() error {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	// Create SaaS extension immediately
	if err := i.startKolideSaasExtension(ctx); err != nil {
		traces.SetError(span, fmt.Errorf("could not create Kolide SaaS extension: %w", err))
		return fmt.Errorf("creating Kolide SaaS extension: %w", err)
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(i.knapsack.RootDirectory(), i.registrationId, i.runId, i.opts)
	if err != nil {
		traces.SetError(span, fmt.Errorf("could not calculate osquery file paths: %w", err))
		return fmt.Errorf("could not calculate osquery file paths: %w", err)
	}

	// Populate augeas lenses, if requested
	if i.opts.augeasLensFunc != nil {
		if err := os.MkdirAll(paths.augeasPath, 0755); err != nil {
			traces.SetError(span, fmt.Errorf("making augeas lenses directory: %w", err))
			return fmt.Errorf("making augeas lenses directory: %w", err)
		}

		if err := i.opts.augeasLensFunc(paths.augeasPath); err != nil {
			traces.SetError(span, fmt.Errorf("setting up augeas lenses: %w", err))
			return fmt.Errorf("setting up augeas lenses: %w", err)
		}
	}

	// The knapsack will retrieve the correct version of osqueryd from the download library if available.
	// If not available, it will fall back to the configured installed version of osqueryd.
	currentOsquerydBinaryPath := i.knapsack.LatestOsquerydPath(ctx)
	span.AddEvent("got_osqueryd_binary_path", trace.WithAttributes(attribute.String("path", currentOsquerydBinaryPath)))

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	i.cmd, err = i.createOsquerydCommand(currentOsquerydBinaryPath, paths)
	if err != nil {
		traces.SetError(span, fmt.Errorf("couldn't create osqueryd command: %w", err))
		return fmt.Errorf("couldn't create osqueryd command: %w", err)
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	i.cmd.SysProcAttr = setpgid()

	i.slogger.Log(ctx, slog.LevelInfo,
		"launching osqueryd",
		"path", i.cmd.Path,
		"args", strings.Join(i.cmd.Args, " "),
	)

	// remove any socket already at the extension socket path to ensure
	// that it's not left over from a previous instance
	if err := os.RemoveAll(paths.extensionSocketPath); err != nil {
		i.slogger.Log(ctx, slog.LevelWarn,
			"error removing osquery extension socket",
			"path", paths.extensionSocketPath,
			"err", err,
		)
	}

	// Launch osquery process (async)
	err = i.startFunc(i.cmd)
	if err != nil {
		// Failure here is indicative of a failure to exec. A missing
		// binary? Bad permissions? TODO: Consider catching errors in the
		// update system and falling back to an earlier version.
		msgPairs := append(
			getOsqueryInfoForLog(i.cmd.Path),
			"err", err,
		)

		i.slogger.Log(ctx, slog.LevelWarn,
			"fatal error starting osquery -- could not exec.",
			msgPairs...,
		)
		traces.SetError(span, fmt.Errorf("fatal error starting osqueryd process: %w", err))
		return fmt.Errorf("fatal error starting osqueryd process: %w", err)
	}

	span.AddEvent("launched_osqueryd")
	i.slogger.Log(ctx, slog.LevelInfo,
		"launched osquery process",
		"osqueryd_pid", i.cmd.Process.Pid,
	)

	// wait for osquery to create the socket before moving on,
	// this is intended to serve as a kind of health check
	// for osquery, if it's started successfully it will create
	// a socket
	if err := backoff.WaitFor(func() error {
		_, err := os.Stat(paths.extensionSocketPath)
		if err != nil {
			i.slogger.Log(ctx, slog.LevelDebug,
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
	i.slogger.Log(ctx, slog.LevelDebug,
		"osquery socket created",
	)

	stats, err := history.NewInstance(i.runId)
	if err != nil {
		i.slogger.Log(ctx, slog.LevelWarn,
			"could not create new osquery instance history",
			"err", err,
		)
	}
	i.stats = stats

	// This loop runs in the background when the process was
	// successfully started. ("successful" is independent of exit
	// code. eg: this runs if we could exec. Failure to exec is above.)
	i.addGoroutineToErrgroup(ctx, "monitor_osquery_process", func() error {
		err := i.cmd.Wait()
		switch {
		case err == nil, isExitOk(err):
			i.slogger.Log(ctx, slog.LevelInfo,
				"osquery exited successfully",
			)
			return errors.New("osquery process exited successfully")
		default:
			msgPairs := append(
				getOsqueryInfoForLog(i.cmd.Path),
				"err", err,
			)

			i.slogger.Log(ctx, slog.LevelWarn,
				"error running osquery command",
				msgPairs...,
			)
			return fmt.Errorf("running osqueryd command: %w", err)
		}
	})

	// Kill osquery process on shutdown
	i.addShutdownGoroutineToErrgroup(ctx, "kill_osquery_process", func() error {
		if i.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(i.cmd); err != nil {
				if strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
					i.slogger.Log(ctx, slog.LevelDebug,
						"tried to stop osquery, but process already gone",
					)
				} else {
					i.slogger.Log(ctx, slog.LevelWarn,
						"error killing osquery process",
						"err", err,
					)
				}
			}
		}
		return i.doneCtx.Err()
	})

	// Start an extension manager for the extensions that osquery
	// needs for config/log/etc.
	i.extensionManagerClient, err = i.StartOsqueryClient(paths)
	if err != nil {
		traces.SetError(span, fmt.Errorf("could not create an extension client: %w", err))
		return fmt.Errorf("could not create an extension client: %w", err)
	}
	span.AddEvent("extension_client_created")

	kolideSaasPlugins := []osquery.OsqueryPlugin{
		config.NewPlugin(KolideSaasExtensionName, i.saasExtension.GenerateConfigs),
		distributed.NewPlugin(KolideSaasExtensionName, i.saasExtension.GetQueries, i.saasExtension.WriteResults),
		osquerylogger.NewPlugin(KolideSaasExtensionName, i.saasExtension.LogString),
	}
	kolideSaasPlugins = append(kolideSaasPlugins, table.PlatformTables(i.knapsack, i.knapsack.Slogger().With("component", "platform_tables"), currentOsquerydBinaryPath)...)
	kolideSaasPlugins = append(kolideSaasPlugins, table.LauncherTables(i.knapsack)...)

	if err := i.StartOsqueryExtensionManagerServer(KolideSaasExtensionName, paths.extensionSocketPath, i.extensionManagerClient, kolideSaasPlugins); err != nil {
		i.slogger.Log(ctx, slog.LevelInfo,
			"unable to create Kolide SaaS extension server, stopping",
			"err", err,
		)
		traces.SetError(span, fmt.Errorf("could not create Kolide SaaS extension server: %w", err))
		return fmt.Errorf("could not create Kolide SaaS extension server: %w", err)
	}
	span.AddEvent("extension_server_created")

	// All done with osquery setup! Mark instance as connected, then proceed
	// with setting up remaining errgroups.
	if err := i.stats.Connected(i); err != nil {
		i.slogger.Log(ctx, slog.LevelWarn,
			"could not set connection time for osquery instance history",
			"err", err,
		)
	}

	// Health check on interval
	i.addGoroutineToErrgroup(ctx, "healthcheck", func() error {
		if i.knapsack != nil && i.knapsack.OsqueryHealthcheckStartupDelay() != 0*time.Second {
			i.slogger.Log(ctx, slog.LevelDebug,
				"entering delay before starting osquery healthchecks",
			)
			select {
			case <-time.After(i.knapsack.OsqueryHealthcheckStartupDelay()):
				i.slogger.Log(ctx, slog.LevelDebug,
					"exiting delay before starting osquery healthchecks",
				)
			case <-i.doneCtx.Done():
				return i.doneCtx.Err()
			}
		}

		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-i.doneCtx.Done():
				return i.doneCtx.Err()
			case <-ticker.C:
				// If device is sleeping, we do not want to perform unnecessary healthchecks that
				// may force an unnecessary restart.
				if i.knapsack != nil && i.knapsack.InModernStandby() {
					break
				}

				// Health check! Allow a couple
				// failures before we tear everything
				// down. This is pretty simple, it
				// hardcodes the timing. Might be
				// better for a Limiter
				maxHealthChecks := 5
				for idx := 1; idx <= maxHealthChecks; idx++ {
					err := i.Healthy()
					if err == nil {
						// err was nil, clear failed attempts
						if idx > 1 {
							i.slogger.Log(ctx, slog.LevelDebug,
								"healthcheck passed, clearing error",
								"attempt", idx,
							)
						}
						break
					}

					if idx == maxHealthChecks {
						i.slogger.Log(ctx, slog.LevelInfo,
							"healthcheck failed, giving up",
							"attempt", idx,
							"err", err,
						)
						return fmt.Errorf("health check failed: %w", err)
					}

					i.slogger.Log(ctx, slog.LevelDebug,
						"healthcheck failed, will retry",
						"attempt", idx,
						"err", err,
					)
					time.Sleep(1 * time.Second)
				}
			}
		}
	})

	// Clean up PID file on shutdown
	i.addShutdownGoroutineToErrgroup(ctx, "remove_pid_file", func() error {
		// We do a couple retries -- on Windows, the PID file may still be in use
		// and therefore unable to be removed.
		if err := backoff.WaitFor(func() error {
			if err := os.Remove(paths.pidfilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing PID file: %w", err)
			}
			return nil
		}, 5*time.Second, 500*time.Millisecond); err != nil {
			i.slogger.Log(ctx, slog.LevelInfo,
				"could not remove PID file, despite retries",
				"pid_file", paths.pidfilePath,
				"err", err,
			)
		}
		return i.doneCtx.Err()
	})

	// Clean up socket file on shutdown
	i.addShutdownGoroutineToErrgroup(ctx, "remove_socket_file", func() error {
		// We do a couple retries -- on Windows, the socket file may still be in use
		// and therefore unable to be removed.
		if err := backoff.WaitFor(func() error {
			if err := os.Remove(paths.extensionSocketPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing socket file: %w", err)
			}
			return nil
		}, 5*time.Second, 500*time.Millisecond); err != nil {
			i.slogger.Log(ctx, slog.LevelInfo,
				"could not remove socket file, despite retries",
				"socket_file", paths.extensionSocketPath,
				"err", err,
			)
		}
		return i.doneCtx.Err()
	})

	return nil
}

// startKolideSaasExtension creates the Kolide SaaS extension, which provides configuration,
// distributed queries, and a log destination for the osquery process.
func (i *OsqueryInstance) startKolideSaasExtension(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// create the osquery extension
	extOpts := launcherosq.ExtensionOpts{
		LoggingInterval: i.knapsack.LoggingInterval(),
	}

	// Setting MaxBytesPerBatch is a tradeoff. If it's too low, we
	// can never send a large result. But if it's too high, we may
	// not be able to send the data over a low bandwidth
	// connection before the connection is timed out.
	//
	// The logic for setting this is spread out. The underlying
	// extension defaults to 3mb, to support GRPC's hardcoded 4MB
	// limit. But as we're transport aware here. we can set it to
	// 5MB for others.
	if i.knapsack.LogMaxBytesPerBatch() != 0 {
		if i.knapsack.Transport() == "grpc" && i.knapsack.LogMaxBytesPerBatch() > 3 {
			i.slogger.Log(ctx, slog.LevelInfo,
				"LogMaxBytesPerBatch is set above the grpc recommended maximum of 3. Expect errors",
				"log_max_bytes_per_batch", i.knapsack.LogMaxBytesPerBatch(),
			)
		}
		extOpts.MaxBytesPerBatch = i.knapsack.LogMaxBytesPerBatch() << 20
	} else if i.knapsack.Transport() == "grpc" {
		extOpts.MaxBytesPerBatch = 3 << 20
	} else if i.knapsack.Transport() != "grpc" {
		extOpts.MaxBytesPerBatch = 5 << 20
	}

	// Create the extension
	var err error
	i.saasExtension, err = launcherosq.NewExtension(ctx, i.serviceClient, i.knapsack, extOpts)
	if err != nil {
		return fmt.Errorf("creating new extension: %w", err)
	}

	// Immediately attempt enrollment in the background. We don't want to put this in our errgroup
	// because we don't need to shut down the whole instance if we can't enroll -- we can always
	// retry later.
	gowrapper.Go(ctx, i.slogger, func() {
		_, nodeInvalid, err := i.saasExtension.Enroll(ctx)
		if nodeInvalid || err != nil {
			i.slogger.Log(ctx, slog.LevelWarn,
				"could not perform initial attempt at enrollment, will retry later",
				"node_invalid", nodeInvalid,
				"err", err,
			)
		}
	}, func(r any) {})

	// Run extension
	i.addGoroutineToErrgroup(ctx, "saas_extension_execute", func() error {
		if err := i.saasExtension.Execute(); err != nil {
			return fmt.Errorf("kolide_grpc extension returned error: %w", err)
		}
		return nil
	})

	// Register shutdown group for extension
	i.addShutdownGoroutineToErrgroup(ctx, "saas_extension_cleanup", func() error {
		i.saasExtension.Shutdown(i.doneCtx.Err())
		return i.doneCtx.Err()
	})

	return nil
}

// addGoroutineToErrgroup adds the given goroutine to the errgroup, ensuring that we log its start and exit.
func (i *OsqueryInstance) addGoroutineToErrgroup(ctx context.Context, goroutineName string, goroutine func() error) {
	i.errgroup.Go(func() error {
		defer i.slogger.Log(ctx, slog.LevelInfo,
			"exiting goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		i.slogger.Log(ctx, slog.LevelInfo,
			"starting goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		return goroutine()
	})
}

// addShutdownGoroutineToErrgroup adds the given goroutine to the errgroup, ensuring that we log its start and exit.
// The goroutine will not execute until the instance has received a signal to exit.
func (i *OsqueryInstance) addShutdownGoroutineToErrgroup(ctx context.Context, goroutineName string, goroutine func() error) {
	i.errgroup.Go(func() error {
		defer i.slogger.Log(ctx, slog.LevelInfo,
			"exiting shutdown goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		// Wait for errgroup to exit
		<-i.doneCtx.Done()

		i.slogger.Log(ctx, slog.LevelInfo,
			"starting shutdown goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		return goroutine()
	})
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	augeasPath            string
	databasePath          string
	extensionAutoloadPath string
	extensionSocketPath   string
	pidfilePath           string
}

// calculateOsqueryPaths accepts a path to a working osqueryd binary and a root
// directory where all of the osquery filesystem artifacts should be stored.
// In return, a structure of paths is returned that can be used to launch an
// osqueryd instance. An error may be returned if the supplied parameters are
// unacceptable.
func calculateOsqueryPaths(rootDirectory string, registrationId string, runId string, opts osqueryOptions) (*osqueryFilePaths, error) {

	// Determine the path to the extension socket
	extensionSocketPath := opts.extensionSocketPath
	if extensionSocketPath == "" {
		extensionSocketPath = SocketPath(rootDirectory, runId)
	}

	extensionAutoloadPath := filepath.Join(rootDirectory, "osquery.autoload")

	// We want to use a unique pidfile per launcher run to avoid file locking issues.
	// See: https://github.com/kolide/launcher/issues/1599
	osqueryFilePaths := &osqueryFilePaths{
		pidfilePath:           filepath.Join(rootDirectory, fmt.Sprintf("osquery-%s.pid", runId)),
		databasePath:          filepath.Join(rootDirectory, fmt.Sprintf("osquery-%s.db", registrationId)),
		augeasPath:            filepath.Join(rootDirectory, "augeas-lenses"),
		extensionSocketPath:   extensionSocketPath,
		extensionAutoloadPath: extensionAutoloadPath,
	}

	// Keep default database path for default instance
	if registrationId == defaultRegistrationId {
		osqueryFilePaths.databasePath = filepath.Join(rootDirectory, "osquery.db")
	}

	osqueryAutoloadFile, err := os.Create(extensionAutoloadPath)
	if err != nil {
		return nil, fmt.Errorf("creating autoload file: %w", err)
	}
	defer osqueryAutoloadFile.Close()

	return osqueryFilePaths, nil
}

// createOsquerydCommand uses osqueryOptions to return an *exec.Cmd
// which will launch a properly configured osqueryd process.
func (i *OsqueryInstance) createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	args := []string{
		fmt.Sprintf("--logger_plugin=%s", KolideSaasExtensionName),
		fmt.Sprintf("--distributed_plugin=%s", KolideSaasExtensionName),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--host_identifier=uuid",
		"--force=true",
		"--utc",
	}

	if i.knapsack.WatchdogEnabled() {
		args = append(args, fmt.Sprintf("--watchdog_memory_limit=%d", i.knapsack.WatchdogMemoryLimitMB()))
		args = append(args, fmt.Sprintf("--watchdog_utilization_limit=%d", i.knapsack.WatchdogUtilizationLimitPercent()))
		args = append(args, fmt.Sprintf("--watchdog_delay=%d", i.knapsack.WatchdogDelaySec()))
	} else {
		args = append(args, "--disable_watchdog")
	}

	cmd := exec.Command( //nolint:forbidigo // We trust the autoupdate library to find the correct path
		osquerydBinary,
		args...,
	)

	if i.knapsack.OsqueryVerbose() {
		cmd.Args = append(cmd.Args, "--verbose")
	}

	// Configs aren't expected to change often, so refresh configs
	// every couple minutes. if there's a failure, try again more
	// promptly. Values in seconds. These settings are CLI flags only.
	cmd.Args = append(cmd.Args,
		"--config_refresh=300",
		"--config_accelerated_refresh=30",
	)

	// Augeas. No windows support, and only makes sense if we populated it.
	if paths.augeasPath != "" && runtime.GOOS != "windows" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--augeas_lenses=%s", paths.augeasPath))
	}

	cmd.Args = append(cmd.Args, platformArgs()...)
	if i.opts.stdout != nil {
		cmd.Stdout = i.opts.stdout
	}
	if i.opts.stderr != nil {
		cmd.Stderr = i.opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the flags below)
	for _, flag := range i.knapsack.OsqueryFlags() {
		cmd.Args = append(cmd.Args, "--"+flag)
	}

	// These flags cannot be overridden (to prevent users from breaking Launcher
	// by providing invalid flags)
	cmd.Args = append(
		cmd.Args,
		fmt.Sprintf("--pidfile=%s", paths.pidfilePath),
		fmt.Sprintf("--database_path=%s", paths.databasePath),
		fmt.Sprintf("--extensions_socket=%s", paths.extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", paths.extensionAutoloadPath),
		"--disable_extensions=false",
		"--extensions_timeout=20",
		fmt.Sprintf("--config_plugin=%s", KolideSaasExtensionName),
		fmt.Sprintf("--extensions_require=%s", KolideSaasExtensionName),
	)

	// On darwin, run osquery using a magic macOS variable to ensure we
	// get proper versions strings back. I'm not totally sure why apple
	// did this, but reading SystemVersion.plist is different when this is set.
	// See:
	// https://eclecticlight.co/2020/08/13/macos-version-numbering-isnt-so-simple/
	// https://github.com/osquery/osquery/pull/6824
	cmd.Env = append(cmd.Env, "SYSTEM_VERSION_COMPAT=0")

	return cmd, nil
}

// StartOsqueryClient will create and return a new osquery client with a connection
// over the socket at the provided path. It will retry for up to 10 seconds to create
// the connection in the event of a failure.
func (i *OsqueryInstance) StartOsqueryClient(paths *osqueryFilePaths) (*osquery.ExtensionManagerClient, error) {
	var client *osquery.ExtensionManagerClient
	if err := backoff.WaitFor(func() error {
		var newErr error
		client, newErr = osquery.NewClient(paths.extensionSocketPath, socketOpenTimeout/2, osquery.DefaultWaitTime(1*time.Second), osquery.MaxWaitTime(maxSocketWaitTime))
		return newErr
	}, socketOpenTimeout, socketOpenInterval); err != nil {
		return nil, fmt.Errorf("could not create an extension client: %w", err)
	}

	return client, nil
}

// startOsqueryExtensionManagerServer takes a set of plugins, creates
// an osquery.NewExtensionManagerServer for them, and then starts it.
func (i *OsqueryInstance) StartOsqueryExtensionManagerServer(name string, socketPath string, client *osquery.ExtensionManagerClient, plugins []osquery.OsqueryPlugin) error {
	var extensionManagerServer *osquery.ExtensionManagerServer
	if err := backoff.WaitFor(func() error {
		var newErr error
		extensionManagerServer, newErr = osquery.NewExtensionManagerServer(
			name,
			socketPath,
			osquery.ServerTimeout(1*time.Minute),
			osquery.WithClient(client),
		)
		return newErr
	}, socketOpenTimeout, socketOpenInterval); err != nil {
		return fmt.Errorf("could not create an extension server: %w", err)
	}

	extensionManagerServer.RegisterPlugin(plugins...)

	i.emsLock.Lock()
	defer i.emsLock.Unlock()

	i.extensionManagerServers = append(i.extensionManagerServers, extensionManagerServer)

	// Start!
	i.addGoroutineToErrgroup(context.TODO(), name, func() error {
		if err := extensionManagerServer.Start(); err != nil {
			i.slogger.Log(context.TODO(), slog.LevelInfo,
				"extension manager server startup got error",
				"err", err,
				"extension_name", name,
			)
			return fmt.Errorf("running extension server: %w", err)
		}

		return errors.New("extension manager server exited")
	})

	// register a shutdown routine
	i.addShutdownGoroutineToErrgroup(context.TODO(), fmt.Sprintf("%s_cleanup", name), func() error {
		if err := extensionManagerServer.Shutdown(context.TODO()); err != nil {
			i.slogger.Log(context.TODO(), slog.LevelInfo,
				"got error while shutting down extension server",
				"err", err,
				"extension_name", name,
			)
		}
		return i.doneCtx.Err()
	})

	return nil
}

// getOsqueryInfoForLog will log info about an osquery instance. It's
// called when osquery unexpected fails to start. (returns as an
// interface for go-kit's logger)
func getOsqueryInfoForLog(path string) []interface{} {
	msgPairs := []interface{}{
		"path", path,
	}

	file, err := os.Open(path)
	if err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("opening file: %w", err))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("stat file: %w", err))
	}

	msgPairs = append(
		msgPairs,
		"sizeBytes", fileInfo.Size(),
		"mode", fileInfo.Mode(),
	)

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("hashing file: %w", err))
	}

	msgPairs = append(
		msgPairs,
		"sha256", fmt.Sprintf("%x", sum.Sum(nil)),
	)

	return msgPairs
}
