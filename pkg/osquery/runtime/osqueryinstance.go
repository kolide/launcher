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
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquerylogger "github.com/osquery/osquery-go/plugin/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"golang.org/x/sync/errgroup"
)

// OsqueryInstanceOption is a functional option pattern for defining how an
// osqueryd instance should be configured. For more information on this pattern,
// see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type OsqueryInstanceOption func(*OsqueryInstance)

// WithOsqueryExtensionPlugins is a functional option which allows the user to
// declare a number of osquery plugins (ie: config plugin, logger plugin, tables,
// etc) which can be loaded when calling LaunchOsqueryInstance. You can load as
// many plugins as you'd like.
func WithOsqueryExtensionPlugins(plugins ...osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.extensionPlugins = append(i.opts.extensionPlugins, plugins...)
	}
}

// WithExtensionSocketPath is a functional option which allows the user to
// define the path of the extension socket path that osqueryd will open to
// communicate with other processes.
func WithExtensionSocketPath(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.extensionSocketPath = path
	}
}

// WithConfigPluginFlag is a functional option which allows the user to define
// which config plugin osqueryd should use to retrieve the config. If this is not
// defined, it is assumed that no configuration is needed and a no-op config
// will be used. This should only be configured once and cannot be changed once
// osqueryd is running.
func WithConfigPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.configPluginFlag = plugin
	}
}

// WithLoggerPluginFlag is a functional option which allows the user to define
// which logger plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default logger. The
// logger plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithLoggerPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.loggerPluginFlag = plugin
	}
}

// WithDistributedPluginFlag is a functional option which allows the user to define
// which distributed plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default distributed. The
// distributed plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithDistributedPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.distributedPluginFlag = plugin
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
	opts     osqueryOptions
	knapsack types.Knapsack
	slogger  *slog.Logger
	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	errgroup                *errgroup.Group
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
func (o *OsqueryInstance) Healthy() error {
	// Do not add/remove servers from o.extensionManagerServers while we're accessing them
	o.emsLock.RLock()
	defer o.emsLock.RUnlock()

	if len(o.extensionManagerServers) == 0 || o.extensionManagerClient == nil {
		return errors.New("instance not started")
	}

	for _, srv := range o.extensionManagerServers {
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

	clientStatus, err := o.extensionManagerClient.Ping()
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

func (o *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	if o.extensionManagerClient == nil {
		return nil, errors.New("client not ready")
	}

	resp, err := o.extensionManagerClient.QueryContext(ctx, query)
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
	augeasLensFunc        func(dir string) error
	configPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	extensionSocketPath   string
	loggerPluginFlag      string
	stderr                io.Writer
	stdout                io.Writer
}

// requiredExtensions returns a unique list of external
// extensions. These are extensions we expect osquery to pause start
// for.
func (o osqueryOptions) requiredExtensions() []string {
	extensionsMap := make(map[string]bool)
	requiredExtensions := make([]string, 0)

	for _, extension := range []string{o.loggerPluginFlag, o.configPluginFlag, o.distributedPluginFlag} {
		// skip the osquery build-ins, since requiring them will cause osquery to needlessly wait.
		if extension == "tls" {
			continue
		}

		if _, ok := extensionsMap[extension]; ok {
			continue
		}

		extensionsMap[extension] = true
		requiredExtensions = append(requiredExtensions, extension)
	}

	return requiredExtensions
}

func newInstance(knapsack types.Knapsack, opts ...OsqueryInstanceOption) *OsqueryInstance {
	i := &OsqueryInstance{
		knapsack: knapsack,
		slogger:  knapsack.Slogger().With("component", "osquery_instance"),
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

func (o *OsqueryInstance) launch() error {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(o.knapsack.RootDirectory(), o.opts)
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

	o.slogger.Log(ctx, slog.LevelInfo,
		"launching osqueryd",
		"path", o.cmd.Path,
		"args", strings.Join(o.cmd.Args, " "),
	)

	// remove any socket already at the extension socket path to ensure
	// that it's not left over from a previous instance
	if err := os.RemoveAll(paths.extensionSocketPath); err != nil {
		o.slogger.Log(ctx, slog.LevelWarn,
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

		o.slogger.Log(ctx, slog.LevelWarn,
			"fatal error starting osquery -- could not exec.",
			msgPairs...,
		)
		traces.SetError(span, fmt.Errorf("fatal error starting osqueryd process: %w", err))
		return fmt.Errorf("fatal error starting osqueryd process: %w", err)
	}

	span.AddEvent("launched_osqueryd")
	o.slogger.Log(ctx, slog.LevelInfo,
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
			o.slogger.Log(ctx, slog.LevelDebug,
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
	o.slogger.Log(ctx, slog.LevelDebug,
		"osquery socket created",
	)

	stats, err := history.NewInstance()
	if err != nil {
		o.slogger.Log(ctx, slog.LevelWarn,
			"could not create new osquery instance history",
			"err", err,
		)
	}
	o.stats = stats

	// This loop runs in the background when the process was
	// successfully started. ("successful" is independent of exit
	// code. eg: this runs if we could exec. Failure to exec is above.)
	o.errgroup.Go(func() error {
		defer o.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "monitor osquery process",
		)

		err := o.cmd.Wait()
		switch {
		case err == nil, isExitOk(err):
			o.slogger.Log(ctx, slog.LevelInfo,
				"osquery exited successfully",
			)
			// TODO: should this return nil?
			return errors.New("osquery process exited successfully")
		default:
			msgPairs := append(
				getOsqueryInfoForLog(o.cmd.Path),
				"err", err,
			)

			o.slogger.Log(ctx, slog.LevelWarn,
				"error running osquery command",
				msgPairs...,
			)
			return fmt.Errorf("running osqueryd command: %w", err)
		}
	})

	// Kill osquery process on shutdown
	o.errgroup.Go(func() error {
		defer o.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "kill osquery process on shutdown",
		)

		<-o.doneCtx.Done()
		o.slogger.Log(ctx, slog.LevelDebug,
			"starting osquery shutdown",
		)
		if o.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(o.cmd); err != nil {
				if strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
					o.slogger.Log(ctx, slog.LevelDebug,
						"tried to stop osquery, but process already gone",
					)
				} else {
					o.slogger.Log(ctx, slog.LevelWarn,
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
			o.slogger.Log(ctx, slog.LevelInfo,
				"unable to create initial extension server, stopping",
				"err", err,
			)
			traces.SetError(span, fmt.Errorf("could not create an extension server: %w", err))
			return fmt.Errorf("could not create an extension server: %w", err)
		}
		span.AddEvent("extension_server_created")
	}

	// Now spawn an extension manager for the tables. We need to
	// start this one in the background, because the runner.Start
	// function needs to return promptly enough for osquery to use
	// it to enroll. Very racy
	//
	// TODO: Consider chunking, if we find we can only have so
	// many tables per extension manager
	o.errgroup.Go(func() error {
		defer o.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "kolide extension manager server launch",
		)

		plugins := table.PlatformTables(o.knapsack, o.knapsack.Slogger().With("component", "platform_tables"), currentOsquerydBinaryPath)

		if len(plugins) == 0 {
			return nil
		}

		if err := o.StartOsqueryExtensionManagerServer("kolide", paths.extensionSocketPath, o.extensionManagerClient, plugins); err != nil {
			o.slogger.Log(ctx, slog.LevelWarn,
				"unable to create tables extension server, stopping",
				"err", err,
			)
			return fmt.Errorf("could not create a table extension server: %w", err)
		}
		return nil
	})

	// All done with osquery setup! Mark instance as connected, then proceed
	// with setting up remaining errgroups.
	if err := o.stats.Connected(o); err != nil {
		o.slogger.Log(ctx, slog.LevelWarn,
			"could not set connection time for osquery instance history",
			"err", err,
		)
	}

	// Health check on interval
	o.errgroup.Go(func() error {
		defer o.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "health check on interval",
		)

		if o.knapsack != nil && o.knapsack.OsqueryHealthcheckStartupDelay() != 0*time.Second {
			o.slogger.Log(ctx, slog.LevelDebug,
				"entering delay before starting osquery healthchecks",
			)
			select {
			case <-time.After(o.knapsack.OsqueryHealthcheckStartupDelay()):
				o.slogger.Log(ctx, slog.LevelDebug,
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
					err := o.Healthy()
					if err == nil {
						// err was nil, clear failed attempts
						if i > 1 {
							o.slogger.Log(ctx, slog.LevelDebug,
								"healthcheck passed, clearing error",
								"attempt", i,
							)
						}
						break
					}

					if i == maxHealthChecks {
						o.slogger.Log(ctx, slog.LevelInfo,
							"healthcheck failed, giving up",
							"attempt", i,
							"err", err,
						)
						return fmt.Errorf("health check failed: %w", err)
					}

					o.slogger.Log(ctx, slog.LevelDebug,
						"healthcheck failed, will retry",
						"attempt", i,
						"err", err,
					)
					time.Sleep(1 * time.Second)
				}
			}
		}
	})

	// Clean up PID file on shutdown
	o.errgroup.Go(func() error {
		defer o.slogger.Log(ctx, slog.LevelInfo,
			"exiting errgroup",
			"errgroup", "cleanup PID file",
		)

		<-o.doneCtx.Done()
		// We do a couple retries -- on Windows, the PID file may still be in use
		// and therefore unable to be removed.
		if err := backoff.WaitFor(func() error {
			if err := os.Remove(paths.pidfilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing PID file: %w", err)
			}
			return nil
		}, 5*time.Second, 500*time.Millisecond); err != nil {
			o.slogger.Log(ctx, slog.LevelInfo,
				"could not remove PID file, despite retries",
				"pid_file", paths.pidfilePath,
				"err", err,
			)
		}
		return o.doneCtx.Err()
	})

	return nil
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
func calculateOsqueryPaths(rootDirectory string, opts osqueryOptions) (*osqueryFilePaths, error) {

	// Determine the path to the extension socket
	extensionSocketPath := opts.extensionSocketPath
	if extensionSocketPath == "" {
		extensionSocketPath = SocketPath(rootDirectory)
	}

	extensionAutoloadPath := filepath.Join(rootDirectory, "osquery.autoload")

	// We want to use a unique pidfile per launcher run to avoid file locking issues.
	// See: https://github.com/kolide/launcher/issues/1599
	osqueryFilePaths := &osqueryFilePaths{
		pidfilePath:           filepath.Join(rootDirectory, fmt.Sprintf("osquery-%s.pid", ulid.New())),
		databasePath:          filepath.Join(rootDirectory, "osquery.db"),
		augeasPath:            filepath.Join(rootDirectory, "augeas-lenses"),
		extensionSocketPath:   extensionSocketPath,
		extensionAutoloadPath: extensionAutoloadPath,
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
func (o *OsqueryInstance) createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	args := []string{
		fmt.Sprintf("--logger_plugin=%s", o.opts.loggerPluginFlag),
		fmt.Sprintf("--distributed_plugin=%s", o.opts.distributedPluginFlag),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--host_identifier=uuid",
		"--force=true",
		"--utc",
	}

	if o.knapsack.WatchdogEnabled() {
		args = append(args, fmt.Sprintf("--watchdog_memory_limit=%d", o.knapsack.WatchdogMemoryLimitMB()))
		args = append(args, fmt.Sprintf("--watchdog_utilization_limit=%d", o.knapsack.WatchdogUtilizationLimitPercent()))
		args = append(args, fmt.Sprintf("--watchdog_delay=%d", o.knapsack.WatchdogDelaySec()))
	} else {
		args = append(args, "--disable_watchdog")
	}

	cmd := exec.Command( //nolint:forbidigo // We trust the autoupdate library to find the correct path
		osquerydBinary,
		args...,
	)

	if o.knapsack.OsqueryVerbose() {
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
	if o.opts.stdout != nil {
		cmd.Stdout = o.opts.stdout
	}
	if o.opts.stderr != nil {
		cmd.Stderr = o.opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the flags below)
	for _, flag := range o.knapsack.OsqueryFlags() {
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
		fmt.Sprintf("--config_plugin=%s", o.opts.configPluginFlag),
		fmt.Sprintf("--extensions_require=%s", strings.Join(o.opts.requiredExtensions(), ",")),
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
func (o *OsqueryInstance) StartOsqueryClient(paths *osqueryFilePaths) (*osquery.ExtensionManagerClient, error) {
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
func (o *OsqueryInstance) StartOsqueryExtensionManagerServer(name string, socketPath string, client *osquery.ExtensionManagerClient, plugins []osquery.OsqueryPlugin) error {
	o.slogger.Log(context.TODO(), slog.LevelDebug,
		"starting startOsqueryExtensionManagerServer",
		"extension_name", name,
	)

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
		o.slogger.Log(context.TODO(), slog.LevelDebug,
			"could not create an extension server",
			"extension_name", name,
			"err", err,
		)
		return fmt.Errorf("could not create an extension server: %w", err)
	}

	extensionManagerServer.RegisterPlugin(plugins...)

	o.emsLock.Lock()
	defer o.emsLock.Unlock()

	o.extensionManagerServers = append(o.extensionManagerServers, extensionManagerServer)

	// Start!
	o.errgroup.Go(func() error {
		defer o.slogger.Log(context.TODO(), slog.LevelDebug,
			"exiting errgroup",
			"errgroup", "run extension manager server",
			"extension_name", name,
		)

		if err := extensionManagerServer.Start(); err != nil {
			o.slogger.Log(context.TODO(), slog.LevelInfo,
				"extension manager server startup got error",
				"err", err,
				"extension_name", name,
			)
			return fmt.Errorf("running extension server: %w", err)
		}
		return errors.New("extension manager server exited")
	})

	// register a shutdown routine
	o.errgroup.Go(func() error {
		defer o.slogger.Log(context.TODO(), slog.LevelDebug,
			"exiting errgroup",
			"errgroup", "shut down extension manager server",
			"extension_name", name,
		)

		<-o.doneCtx.Done()

		o.slogger.Log(context.TODO(), slog.LevelDebug,
			"starting extension shutdown",
			"extension_name", name,
		)

		if err := extensionManagerServer.Shutdown(context.TODO()); err != nil {
			o.slogger.Log(context.TODO(), slog.LevelInfo,
				"got error while shutting down extension server",
				"err", err,
				"extension_name", name,
			)
		}
		return o.doneCtx.Err()
	})

	o.slogger.Log(context.TODO(), slog.LevelDebug,
		"clean finish startOsqueryExtensionManagerServer",
		"extension_name", name,
	)

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
