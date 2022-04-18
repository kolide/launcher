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
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/osquery/table"

	"github.com/kolide/launcher/pkg/osquery/runtime/history"
)

type Runner struct {
	instance     *OsqueryInstance
	instanceLock sync.Mutex
	shutdown     chan struct{}
}

func (r *Runner) Query(query string) ([]map[string]string, error) {
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()
	return r.instance.Query(query)
}

type osqueryOptions struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	augeasLensFunc        func(dir string) error
	binaryPath            string
	configPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	extensionSocketPath   string
	enrollSecretPath      string
	loggerPluginFlag      string
	osqueryFlags          []string
	retries               uint
	rootDirectory         string
	stderr                io.Writer
	stdout                io.Writer
	tlsConfigEndpoint     string
	tlsDistReadEndpoint   string
	tlsDistWriteEndpoint  string
	tlsEnrollEndpoint     string
	tlsHostname           string
	tlsLoggerEndpoint     string
	tlsServerCerts        string
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
	stats                  *history.Instance
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	augeasPath            string
	databasePath          string
	extensionAutoloadPath string
	extensionPath         string
	extensionSocketPath   string
	pidfilePath           string
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
		augeasPath:            filepath.Join(rootDir, "augeas-lenses"),
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

	if opts.verbose {
		cmd.Args = append(cmd.Args, "--verbose")
	}

	if opts.tlsHostname != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_hostname=%s", opts.tlsHostname))
	}

	if opts.tlsConfigEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--config_tls_endpoint=%s", opts.tlsConfigEndpoint))

	}

	if opts.tlsEnrollEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_tls_endpoint=%s", opts.tlsEnrollEndpoint))
	}

	if opts.tlsLoggerEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--logger_tls_endpoint=%s", opts.tlsLoggerEndpoint))
	}

	if opts.tlsDistReadEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_read_endpoint=%s", opts.tlsDistReadEndpoint))
	}

	if opts.tlsDistWriteEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_write_endpoint=%s", opts.tlsDistWriteEndpoint))
	}

	if opts.tlsServerCerts != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_server_certs=%s", opts.tlsServerCerts))
	}

	if opts.enrollSecretPath != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_secret_path=%s", opts.enrollSecretPath))
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
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	}
	if opts.stderr != nil {
		cmd.Stderr = opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the flags below)
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
		"--disable_extensions=false",
		"--extensions_timeout=20",
		fmt.Sprintf("--config_plugin=%s", opts.configPluginFlag),
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

func WithEnrollSecretPath(secretPath string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.enrollSecretPath = secretPath
	}
}

func WithTlsHostname(hostname string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsHostname = hostname
	}
}

func WithTlsConfigEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsConfigEndpoint = ep
	}
}

func WithTlsEnrollEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsEnrollEndpoint = ep
	}
}

func WithTlsLoggerEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsLoggerEndpoint = ep
	}
}

func WithTlsDistributedReadEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsDistReadEndpoint = ep
	}
}

func WithTlsDistributedWriteEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsDistWriteEndpoint = ep
	}
}

func WithTlsServerCerts(s string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsServerCerts = s
	}
}

// WithOsqueryFlags sets additional flags to pass to osquery
func WithOsqueryFlags(flags []string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.osqueryFlags = flags
	}
}

// WithAugeasLensFunction defines a callback function. This can be
// used during setup to populate the augeas lenses directory.
func WithAugeasLensFunction(f func(dir string) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.augeasLensFunc = f
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
	r.instanceLock.Lock()
	defer r.instanceLock.Unlock()

	return r.instance.Healthy()
}

// timeout and interval values for the various limiters
const (
	healthyInterval     = 1 * time.Second
	healthyTimeout      = 30 * time.Second
	serverStartInterval = 10 * time.Second
	serverStartTimeout  = 5 * time.Minute
	socketOpenInterval  = 10 * time.Second
	socketOpenTimeout   = 5 * time.Minute
)

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
				r.instance.stats.Exited(nil)
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

			r.instance.stats.Exited(err)

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

	// The extensions file should be owned by the process's UID or by root.
	// Osquery will refuse to load the extension otherwise.
	if err := ensureProperPermissions(o, paths.extensionPath); err != nil {
		level.Info(o.logger).Log(
			"msg", "unable to ensure proper permissions on extension path",
			"err", err,
		)
	}

	// Populate augeas lenses, if requested
	if o.opts.augeasLensFunc != nil {
		if err := os.MkdirAll(paths.augeasPath, 0755); err != nil {
			return errors.Wrap(err, "making augeas lenses directory")
		}

		if err := o.opts.augeasLensFunc(paths.augeasPath); err != nil {
			return errors.Wrap(err, "setting up augeas lenses")
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
	// o.runtime stats set start time
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
			return errors.Wrap(err, "running osqueryd command")
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

	// Because we "invert" the control of the osquery process and the
	// extension (we are running the extension from the process that starts
	// osquery, rather than the other way around), we don't know exactly
	// when osquery will have the extension socket open. Because of this,
	// we want to try opening the socket until we are successful (with a
	// timeout if something goes wrong).
	//
	// It would make sense to put a Healthy() check, but that
	// doesn't work -- because osquery is looking for
	// `kolide_grpc` for varius config/logging/etc plugins.
	level.Debug(o.logger).Log("msg", "Starting server connection attempts to osquery")
	WaitFor(func() error {
		o.extensionManagerServer, err = osquery.NewExtensionManagerServer(
			"kolide",
			paths.extensionSocketPath,
			osquery.ServerTimeout(30*time.Second),
		)
		return err
	}, socketOpenTimeout, socketOpenInterval)
	level.Debug(o.logger).Log("msg", "Successfully connected server to osquery")

	// Various followup things use the client for queries, so register it first
	o.extensionManagerClient, err = osquery.NewClient(paths.extensionSocketPath, 30*time.Second)
	if err != nil {
		level.Info(o.logger).Log("msg", "Unable to create extension client. Stopping", "err", err)
		return errors.Wrap(err, "could not create an extension client")
	}

	// Register plugins/tables. There's a lurking gotcha here --
	// when using grpc, osquery startup is (somewhat) blocked on
	// the `kolide_grpc` plugin being registered. But, if we have
	// enough tables, the act of registering them all slows down
	// startup. Perhaps future work could be using two
	// extensionManagerServer, or dropping grpc
	plugins := o.opts.extensionPlugins
	for _, t := range table.PlatformTables(o.extensionManagerClient, o.logger, currentOsquerydBinaryPath) {
		plugins = append(plugins, t)
	}
	o.extensionManagerServer.RegisterPlugin(plugins...)

	// Launch the extension manager server asynchronously. Note
	// that this is async, which can cause subtle ordering
	// issues. (This doesn't need a mutex, it's on the server
	// side, and osquery-go handles that)
	o.errgroup.Go(func() error {
		level.Debug(o.logger).Log("msg", "Starting extension manager server")

		if err := WaitFor(o.extensionManagerServer.Start, serverStartTimeout, serverStartInterval); err != nil {
			level.Info(o.logger).Log("msg", "Extension manager server startup got error", "err", err)
			return errors.Wrap(err, "running extension server")
		}
		return errors.New("extension manager server exited")
	})

	// Cleanup extension manager server on shutdown. This pairs
	// with the o.extensionManagerServer.Start above
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		level.Debug(o.logger).Log("msg", "Starting extension shutdown")
		if err := o.extensionManagerServer.Shutdown(context.TODO()); err != nil {
			level.Info(o.logger).Log(
				"msg", "Got error while shutting down extension server",
				"err", err,
			)
		}
		return o.doneCtx.Err()
	})

	// getting stats requires the Client already be instantiated
	if err := o.stats.Connected(o); err != nil {
		level.Info(o.logger).Log("msg", "osquery instance history", "error", err)
	}

	// Health check on interval
	o.errgroup.Go(func() error {
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-o.doneCtx.Done():
				return o.doneCtx.Err()
			case <-ticker.C:
				if err := WaitFor(o.Healthy, healthyTimeout, healthyInterval); err != nil {
					level.Info(o.logger).Log("msg", "Health check failed. Giving up", "err", err)
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

	o.clientLock.Lock()
	defer o.clientLock.Unlock()

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

	// Note to future self -- The thrift libraries here will
	// occasionally return a hard to debug i/o timeout
	// error. Because osquery is single threaded, this can happen
	// if multiple things try to happen at the same time. The
	// thrift library claims to support some timeout/retry
	// functionality, as implemented by passing a context with
	// deadline along, but testing with it, I can't make it
	// work. Same i/o timeout errors. (Note that you'd need to
	// patch it to osquery-go)
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
