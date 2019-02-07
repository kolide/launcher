package runtime

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/table"
)

type Runner struct {
	instance     *OsqueryInstance
	instanceLock sync.RWMutex
	shutdown     chan struct{}
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	return r.instance.Query(query)
}

type osqueryOptions struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	binaryPath            string
	rootDirectory         string
	extensionSocketPath   string
	configPluginFlag      string
	loggerPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	stdout                io.Writer
	stderr                io.Writer
	retries               uint
}

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	opts   osqueryOptions
	logger log.Logger
	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	errgroup               *errgroup.Group
	doneCtx                context.Context
	cancel                 context.CancelFunc
	cmd                    *exec.Cmd
	extensionManagerServer *osquery.ExtensionManagerServer
	extensionManagerClient *osquery.ExtensionManagerClient
	clientLock             sync.Mutex
	paths                  *osqueryFilePaths
	rmRootDirectory        func()
	usingTempDir           bool
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	pidfilePath           string
	databasePath          string
	extensionPath         string
	extensionAutoloadPath string
	extensionSocketPath   string
}

// calculateOsqueryPaths accepts a path to a working osqueryd binary and a root
// directory where all of the osquery filesystem artifacts should be stored.
// In return, a structure of paths is returned that can be used to launch an
// osqueryd instance. An error may be returned if the supplied parameters are
// unacceptable.
func calculateOsqueryPaths(rootDir, extensionSocketPath string) (*osqueryFilePaths, error) {
	// Determine the path to the extension
	exPath, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "finding path of launcher executable")
	}
	extensionPath := filepath.Join(filepath.Dir(exPath), extensionName)
	if _, err := os.Stat(extensionPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "extension path does not exist: %s", extensionPath)
		} else {
			return nil, errors.Wrapf(err, "could not stat extension path")
		}
	}

	// Determine the path to the extension socket
	if extensionSocketPath == "" {
		extensionSocketPath = socketPath(rootDir)
	}

	// Write the autoload file
	extensionAutoloadPath := filepath.Join(rootDir, "osquery.autoload")
	if err := ioutil.WriteFile(extensionAutoloadPath, []byte(extensionPath), 0644); err != nil {
		return nil, errors.Wrap(err, "could not write osquery extension autoload file")
	}

	return &osqueryFilePaths{
		pidfilePath:           filepath.Join(rootDir, "osquery.pid"),
		databasePath:          filepath.Join(rootDir, "osquery.db"),
		extensionPath:         extensionPath,
		extensionAutoloadPath: extensionAutoloadPath,
		extensionSocketPath:   extensionSocketPath,
	}, nil
}

// createOsquerydCommand accepts a structure of relevant file paths relating to
// an osquery instance and returns an *exec.Cmd which will launch a properly
// configured osqueryd process.
func createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths, configPlugin, loggerPlugin, distributedPlugin string, stdout io.Writer, stderr io.Writer) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	cmd := exec.Command(
		osquerydBinary,
		fmt.Sprintf("--pidfile=%s", paths.pidfilePath),
		fmt.Sprintf("--database_path=%s", paths.databasePath),
		fmt.Sprintf("--extensions_socket=%s", paths.extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", paths.extensionAutoloadPath),
		"--extensions_timeout=10",
		fmt.Sprintf("--config_plugin=%s", configPlugin),
		fmt.Sprintf("--logger_plugin=%s", loggerPlugin),
		fmt.Sprintf("--distributed_plugin=%s", distributedPlugin),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--config_refresh=10",
		"--host_identifier=uuid",
		"--force=true",
		"--disable_watchdog",
		"--utc",
	)
	cmd.Args = append(cmd.Args, platformArgs()...)
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}

	return cmd, nil
}

func osqueryTempDir() (string, func(), error) {
	tempPath, err := ioutil.TempDir("", "")
	if err != nil {
		return "", func() {}, errors.Wrap(err, "could not make temp path")
	}

	return tempPath, func() {
		os.Remove(tempPath)
	}, nil
}

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

// WithOsquerydBinary is a functional option which allows the user to define the
// path of the osqueryd binary which will be launched. This should only be called
// once as only one binary will be executed. Defining the path to the osqueryd
// binary is optional. If it is not explicitly defined by the caller, an osqueryd
// binary will be looked for in the current $PATH.
func WithOsquerydBinary(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.binaryPath = path
	}
}

// WithRootDirectory is a functional option which allows the user to define the
// path where filesystem artifacts will be stored. This may include pidfiles,
// RocksDB database files, etc. If this is not defined, a temporary directory
// will be used.
func WithRootDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.rootDirectory = path
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

// WithLogger is a functional option which allows the user to pass a log.Logger
// to be used for logging osquery instance status.
func WithLogger(logger log.Logger) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.logger = logger
	}
}

// Shutdown instructs the runner to permanently stop the running instance (no
// restart will be attempted).
func (r *Runner) Shutdown() error {
	close(r.shutdown)
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	r.instance.cancel()
	if err := r.instance.errgroup.Wait(); err != context.Canceled {
		return errors.Wrap(err, "while shutting down instance")
	}
	return nil
}

// Healthy checks the health of the instance and returns an error describing
// any problem.
func (r *Runner) Healthy() error {
	r.instanceLock.RLock()
	defer r.instanceLock.RUnlock()
	return r.instance.Healthy()
}

// How long to wait before erroring because we cannot open the osquery
// extension socket.
const socketOpenTimeout = 10 * time.Second

// How often to try to open the osquery extension socket
const socketOpenInterval = 200 * time.Millisecond

// LaunchInstance will launch an instance of osqueryd via a very configurable
// API as defined by the various OsqueryInstanceOption functional options. The
// returned instance should be shut down via the Shutdown() method.
// For example, a more customized caller might do something like the following:
//
//   instance, err := LaunchInstance(
//     WithOsquerydBinary("/usr/local/bin/osqueryd"),
//     WithRootDirectory("/var/foobar"),
//     WithConfigPluginFlag("custom"),
// 		 WithOsqueryExtensionPlugins(
//		 	 config.NewPlugin("custom", custom.GenerateConfigs),
//		   logger.NewPlugin("custom", custom.LogString),
//		 	 tables.NewPlugin("foobar", custom.FoobarColumns, custom.FoobarGenerate),
//     ),
//   )
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

func newInstance() *OsqueryInstance {
	i := &OsqueryInstance{}

	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel
	i.errgroup, i.doneCtx = errgroup.WithContext(ctx)

	i.logger = log.NewNopLogger()

	return i
}

func (r *Runner) Start() error {
	if err := r.launchOsqueryInstance(); err != nil {
		return errors.Wrap(err, "starting instance")
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

const healthCheckInterval = 60 * time.Second

func (r *Runner) launchOsqueryInstance() error {
	o := r.instance
	// If the path of the osqueryd binary wasn't explicitly defined by the caller,
	// try to find it in the path.
	if o.opts.binaryPath == "" {
		path, err := exec.LookPath("osqueryd")
		if err != nil {
			return errors.Wrap(err, "osqueryd not supplied and not found")
		}
		o.opts.binaryPath = path
	}

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if o.opts.rootDirectory == "" {
		rootDirectory, rmRootDirectory, err := osqueryTempDir()
		if err != nil {
			return errors.Wrap(err, "couldn't create temp directory for osquery instance")
		}
		o.opts.rootDirectory = rootDirectory
		o.rmRootDirectory = rmRootDirectory
		o.usingTempDir = true
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(o.opts.rootDirectory, o.opts.extensionSocketPath)
	if err != nil {
		return errors.Wrap(err, "could not calculate osquery file paths")
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

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	o.cmd, err = createOsquerydCommand(o.opts.binaryPath, paths, o.opts.configPluginFlag, o.opts.loggerPluginFlag, o.opts.distributedPluginFlag, o.opts.stdout, o.opts.stderr)
	if err != nil {
		return errors.Wrap(err, "couldn't create osqueryd command")
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	o.cmd.SysProcAttr = setpgid()

	// Launch osquery process (async)
	err = o.cmd.Start()
	if err != nil {
		return errors.Wrap(err, "starting osqueryd process")
	}
	o.errgroup.Go(func() error {
		err := o.cmd.Wait()
		switch {
		case err == nil, isExitOk(err):
			// TODO: should this return nil?
			return errors.New("osquery process exited")
		default:
			return errors.Wrap(err, "running osqueryd command")
		}
	})

	// Kill osquery process on shutdown
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		if o.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(o.cmd); err != nil {
				if !strings.Contains(err.Error(), "process already finished") {
					level.Info(o.logger).Log(
						"msg", "killing osquery process",
						"err", err,
					)
				}
			}
		}
		return o.doneCtx.Err()
	})

	// Because we "invert" the control of the osquery process and the
	// extension (we are running the extension from the process that starts
	// osquery, rather than the other way around), we don't know exactly
	// when osquery will have the extension socket open. Because of this,
	// we want to try opening the socket until we are successful (with a
	// timeout if something goes wrong).
	deadlineCtx, cancel := context.WithTimeout(context.Background(), socketOpenTimeout)
	defer cancel()
	limiter := rate.NewLimiter(rate.Every(socketOpenInterval), 1)
	for {
		// Create the extension server and register all custom osquery
		// plugins
		o.extensionManagerServer, err = osquery.NewExtensionManagerServer(
			"kolide",
			paths.extensionSocketPath,
			osquery.ServerTimeout(2*time.Second),
		)
		if err == nil {
			break
		}

		if limiter.Wait(deadlineCtx) != nil {
			// This means that our timeout expired. Return the
			// error from creating the server, not the error from
			// the timeout expiration.
			return errors.Wrapf(err, "could not create extension manager server at %s", paths.extensionSocketPath)
		}
	}

	o.extensionManagerClient, err = osquery.NewClient(paths.extensionSocketPath, 5*time.Second)
	if err != nil {
		return errors.Wrap(err, "could not create an extension client")
	}

	plugins := o.opts.extensionPlugins
	for _, t := range table.PlatformTables(o.extensionManagerClient, o.logger) {
		plugins = append(plugins, t)
	}
	o.extensionManagerServer.RegisterPlugin(plugins...)

	// Launch the extension manager server asynchronously.
	o.errgroup.Go(func() error {
		// We see the extention manager being slow to start. Implement a simple re-try routine
		backoff := backoff.New()
		if err := backoff.Run(o.extensionManagerServer.Start); err != nil {
			return errors.Wrap(err, "running extension server")
		}
		return errors.New("extension manager server exited")
	})

	// Cleanup extension manager server on shutdown
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		if err := o.extensionManagerServer.Shutdown(context.TODO()); err != nil {
			level.Info(o.logger).Log(
				"msg", "shutting down extension server",
				"err", err,
			)
		}
		return o.doneCtx.Err()
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
				if err := o.Healthy(); err != nil {
					return errors.Wrap(err, "health check failed")
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

// Restart allows you to cleanly shutdown the current instance and launch a new
// instance with the same configurations.
func (r *Runner) Restart() error {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	// Cancelling will cause all of the cleanup routines to execute, and a
	// new instance will start.
	r.instance.cancel()
	r.instance.errgroup.Wait()

	return nil
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy. If the instance is healthy, it returns nil.
func (o *OsqueryInstance) Healthy() error {
	if o.extensionManagerServer == nil || o.extensionManagerClient == nil {
		return errors.New("instance not started")
	}

	serverStatus, err := o.extensionManagerServer.Ping(context.TODO())
	if err != nil {
		return errors.Wrap(err, "could not ping extension server")
	}
	if serverStatus.Code != 0 {
		return errors.Errorf("ping extension server returned %d: %s",
			serverStatus.Code,
			serverStatus.Message,
		)
	}

	o.clientLock.Lock()
	defer o.clientLock.Unlock()
	clientStatus, err := o.extensionManagerClient.Ping()
	if err != nil {
		return errors.Wrap(err, "could not ping osquery extension client")
	}
	if clientStatus.Code != 0 {
		return errors.Errorf("ping extension client returned %d: %s",
			serverStatus.Code,
			serverStatus.Message,
		)
	}

	return nil
}

func (o *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	o.clientLock.Lock()
	defer o.clientLock.Unlock()
	resp, err := o.extensionManagerClient.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "could not query the extension manager client")
	}
	if resp.Status.Code != int32(0) {
		return nil, errors.New(resp.Status.Message)
	}

	return resp.Response, nil
}
