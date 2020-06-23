package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
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
	osqueryFlags          []string
	stdout                io.Writer
	stderr                io.Writer
	retries               uint
	verbose               bool
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

	extensionPath := filepath.Join(autoupdate.FindBaseDir(exPath), extensionName)
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

// createOsquerydCommand uses osqueryOptions to return an *exec.Cmd
// which will launch a properly configured osqueryd process.
func (opts *osqueryOptions) createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	cmd := exec.Command(
		osquerydBinary,
		fmt.Sprintf("--logger_plugin=%s", opts.loggerPluginFlag),
		fmt.Sprintf("--distributed_plugin=%s", opts.distributedPluginFlag),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--host_identifier=uuid",
		"--force=true",
		"--disable_watchdog",
		"--utc",
	)

	// TODO: This is a short term hack.
	// In https://github.com/osquery/osquery/pull/6271 osquery
	// shifted some debugging info from INFO to VERBOSE. This has
	// the unfortunate effect of making it hard to correlate
	// distributed query logs with the distributed query that
	// caused them. While we're thinking through the longer term
	// fix, we have a quick mitagation in dropping osquery into
	// verbose mode. (This is duplicative with the opts.verbose
	// parsing, because this whole block should be struck once we
	// have a better approach)
	cmd.Args = append(cmd.Args, "--verbose")

	if opts.verbose {
		cmd.Args = append(cmd.Args, "--verbose")
	}

	// Configs aren't expected to change often, so refresh configs
	// every couple minutes. if there's a failure, try again more
	// promptly. Values in seconds. These settings are CLI flags only.
	cmd.Args = append(cmd.Args,
		"--config_refresh=300",
		"--config_accelerated_refresh=30",
	)

	cmd.Args = append(cmd.Args, platformArgs()...)
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	}
	if opts.stderr != nil {
		cmd.Stderr = opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the six flags below)
	for _, flag := range opts.osqueryFlags {
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
		"--extensions_timeout=10",
		fmt.Sprintf("--config_plugin=%s", opts.configPluginFlag),
	)

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

// WithOsqueryVerbose sets whether or not osquery is in verbose mode
func WithOsqueryVerbose(v bool) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.verbose = v
	}
}

// WithOsqueryFlags sets additional flags to pass to osquery
func WithOsqueryFlags(flags []string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.osqueryFlags = flags
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
		return errors.Wrap(err, "couldn't create osqueryd command")
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	o.cmd.SysProcAttr = setpgid()

	level.Info(o.logger).Log(
		"msg", "launching osqueryd",
		"arg0", o.cmd.Path,
		"args", strings.Join(o.cmd.Args, " "),
	)

	// Launch osquery process (async)
	err = o.cmd.Start()
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
		return errors.Wrap(err, "fatal error starting osqueryd process")
	}

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
			return errors.Wrap(err, "running osqueryd command")
		}
	})

	// Kill osquery process on shutdown
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		if o.cmd.Process != nil {
			// kill osqueryd and children
			if err := killProcessGroup(o.cmd); err != nil {
				if strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
					level.Debug(o.logger).Log("process already gone")
				} else {
					level.Info(o.logger).Log("msg", "killing osquery process", "err", err)
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
	for _, t := range table.PlatformTables(o.extensionManagerClient, o.logger, currentOsquerydBinaryPath) {
		plugins = append(plugins, t)
	}
	o.extensionManagerServer.RegisterPlugin(plugins...)

	// Launch the extension manager server asynchronously.
	o.errgroup.Go(func() error {
		// We see the extension manager being slow to start. Implement a simple re-try routine
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

// getOsqueryInfoForLog will log info about an osquery instance. It's
// called when osquery unexpected fails to start. (returns as an
// interface for go-kit's logger)
func getOsqueryInfoForLog(path string) []interface{} {
	msgPairs := []interface{}{
		"path", path,
	}

	file, err := os.Open(path)
	if err != nil {
		return append(msgPairs, "extraerr", errors.Wrap(err, "opening file"))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return append(msgPairs, "extraerr", errors.Wrap(err, "stat file"))
	}

	msgPairs = append(
		msgPairs,
		"sizeBytes", fileInfo.Size(),
		"mode", fileInfo.Mode(),
	)

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return append(msgPairs, "extraerr", errors.Wrap(err, "hashing file"))
	}

	msgPairs = append(
		msgPairs,
		"sha256", fmt.Sprintf("%x", sum.Sum(nil)),
	)

	return msgPairs
}
