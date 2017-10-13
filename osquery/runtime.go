package osquery

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
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	instance     *osqueryInstanceFields
	instanceLock sync.Mutex
	logger       log.Logger
}

// osqueryInstanceFields is a type which is embedded in OsqueryInstance so that
// in the event that the underlying process instance changes, the fields can be
// updated wholesale without updating the actual OsqueryInstance pointer which
// may be held by the original caller.
type osqueryInstanceFields struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	binaryPath            string
	rootDirectory         string
	configPluginFlag      string
	loggerPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	stdout                io.Writer
	stderr                io.Writer
	retries               uint

	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	cmd                    *exec.Cmd
	errs                   chan error
	extensionManagerServer *osquery.ExtensionManagerServer
	extensionManagerClient *osquery.ExtensionManagerClient
	clientLock             *sync.Mutex
	paths                  *osqueryFilePaths
	hasBegunTeardown       int32
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
func calculateOsqueryPaths(rootDir string) (*osqueryFilePaths, error) {
	// Determine the path to the extension
	exPath, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "finding path of launcher executable")
	}
	extensionPath := filepath.Join(filepath.Dir(exPath), "osquery-extension.ext")
	if _, err := os.Stat(extensionPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "extension path does not exist: %s", extensionPath)
		} else {
			return nil, errors.Wrapf(err, "could not stat extension path")
		}
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
		extensionSocketPath:   filepath.Join(rootDir, "osquery.sock"),
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
	)
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

// WithLogger is a functional option which allows the user to pass a log.Logger
// to be used for logging osquery instance status.
func WithLogger(logger log.Logger) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.logger = logger
	}
}

// WithOsqueryExtensionPlugin is a functional option which allows the user to
// declare a number of osquery plugins (ie: config plugin, logger plugin, tables,
// etc) which can be loaded when calling LaunchOsqueryInstance. You can load as
// many plugins as you'd like.
func WithOsqueryExtensionPlugin(plugin osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.extensionPlugins = append(i.instance.extensionPlugins, plugin)
	}
}

// WithOsquerydBinary is a functional option which allows the user to define the
// path of the osqueryd binary which will be launched. This should only be called
// once as only one binary will be executed. Defining the path to the osqueryd
// binary is optional. If it is not explicitly defined by the caller, an osqueryd
// binary will be looked for in the current $PATH.
func WithOsquerydBinary(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.binaryPath = path
	}
}

// WithRootDirectory is a functional option which allows the user to define the
// path where filesystem artifacts will be stored. This may include pidfiles,
// RocksDB database files, etc. If this is not defined, a temporary directory
// will be used.
func WithRootDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.rootDirectory = path
	}
}

// WithConfigPluginFlag is a functional option which allows the user to define
// which config plugin osqueryd should use to retrieve the config. If this is not
// defined, it is assumed that no configuration is needed and a no-op config
// will be used. This should only be configured once and cannot be changed once
// osqueryd is running.
func WithConfigPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.configPluginFlag = plugin
	}
}

// WithLoggerPluginFlag is a functional option which allows the user to define
// which logger plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default logger. The
// logger plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithLoggerPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.loggerPluginFlag = plugin
	}
}

// WithDistributedPluginFlag is a functional option which allows the user to define
// which distributed plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default distributed. The
// distributed plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithDistributedPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.distributedPluginFlag = plugin
	}
}

// WithStdout is a functional option which allows the user to define where the
// stdout of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStdout(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.stdout = w
	}
}

// WithStderr is a functional option which allows the user to define where the
// stderr of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStderr(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.stderr = w
	}
}

// WithRetries is a functional option which allows the user to define how many
// retries to make when creating the process.
func WithRetries(retries uint) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.instance.retries = retries
	}
}

// How long to wait before erroring because we cannot open the osquery
// extension socket.
const socketOpenTimeout = 5 * time.Second

// How often to try to open the osquery extension socket
const socketOpenInterval = 200 * time.Millisecond

// LaunchOsqueryInstance will launch an instance of osqueryd via a very
// configurable API as defined by the various OsqueryInstanceOption functional
// options. For example, a more customized caller might do something like the
// following:
//
//   instance, err := LaunchOsqueryInstance(
//     WithOsquerydBinary("/usr/local/bin/osqueryd"),
//     WithRootDirectory("/var/foobar"),
//     WithOsqueryExtensionPlugin(config.NewPlugin("custom", custom.GenerateConfigs)),
//     WithConfigPluginFlag("custom"),
//     WithOsqueryExtensionPlugin(logger.NewPlugin("custom", custom.LogString)),
//     WithOsqueryExtensionPlugin(tables.NewPlugin("foobar", custom.FoobarColumns, custom.FoobarGenerate)),
//   )
func LaunchOsqueryInstance(opts ...OsqueryInstanceOption) (*OsqueryInstance, error) {
	// Create an OsqueryInstance and apply the functional options supplied by the
	// caller.
	o := &OsqueryInstance{
		logger: log.NewNopLogger(),
		instance: &osqueryInstanceFields{
			rmRootDirectory: func() {},
			errs:            make(chan error),
			clientLock:      new(sync.Mutex),
		},
	}

	for _, opt := range opts {
		opt(o)
	}

	return launchOsqueryInstanceWithRetry(o)
}

// launchOsqueryInstanceWithRetry wraps launchOsqueryInstance, adding retry
// upon failure.
func launchOsqueryInstanceWithRetry(o *OsqueryInstance) (inst *OsqueryInstance, err error) {
	for try := uint(0); try <= o.instance.retries; try++ {
		inst, err = launchOsqueryInstance(o)
		if err == nil {
			return
		}
	}
	return
}

func launchOsqueryInstance(o *OsqueryInstance) (*OsqueryInstance, error) {
	// If the path of the osqueryd binary wasn't explicitly defined by the caller,
	// try to find it in the path.
	if o.instance.binaryPath == "" {
		path, err := exec.LookPath("osqueryd")
		if err != nil {
			return nil, errors.Wrap(err, "osqueryd not supplied and not found")
		}
		o.instance.binaryPath = path
	}

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if o.instance.rootDirectory == "" {
		rootDirectory, rmRootDirectory, err := osqueryTempDir()
		if err != nil {
			return nil, errors.Wrap(err, "couldn't create temp directory for osquery instance")
		}
		o.instance.rootDirectory = rootDirectory
		o.instance.rmRootDirectory = rmRootDirectory
		o.instance.usingTempDir = true
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(o.instance.rootDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "could not calculate osquery file paths")
	}

	// If a config plugin has not been set by the caller, then it is likely that
	// the instance will just be used for executing queries, so we will use a
	// minimal config plugin that basically is a no-op.
	if o.instance.configPluginFlag == "" {
		generateConfigs := func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		}
		o.instance.extensionPlugins = append(o.instance.extensionPlugins, config.NewPlugin("internal_noop", generateConfigs))
		o.instance.configPluginFlag = "internal_noop"
	}

	// If a logger plugin has not been set by the caller, we set a logger plugin
	// that outputs logs to the default application logger.
	if o.instance.loggerPluginFlag == "" {
		logString := func(ctx context.Context, typ logger.LogType, logText string) error {
			return nil
		}
		o.instance.extensionPlugins = append(o.instance.extensionPlugins, logger.NewPlugin("internal_noop", logString))
		o.instance.loggerPluginFlag = "internal_noop"
	}

	// If a distributed plugin has not been set by the caller, we set a distributed plugin
	// that outputs logs to the default application distributed.
	if o.instance.distributedPluginFlag == "" {
		getQueries := func(ctx context.Context) (*distributed.GetQueriesResult, error) {
			return &distributed.GetQueriesResult{}, nil
		}
		writeResults := func(ctx context.Context, results []distributed.Result) error {
			return nil
		}
		o.instance.extensionPlugins = append(o.instance.extensionPlugins, distributed.NewPlugin("internal_noop", getQueries, writeResults))
		o.instance.distributedPluginFlag = "internal_noop"
	}

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	o.instance.cmd, err = createOsquerydCommand(o.instance.binaryPath, paths, o.instance.configPluginFlag, o.instance.loggerPluginFlag, o.instance.distributedPluginFlag, o.instance.stdout, o.instance.stderr)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create osqueryd command")
	}

	if err := o.instance.cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "couldn't start osqueryd command")
	}

	// Launch a long running goroutine to monitor the osqueryd process.
	cmd := o.instance.cmd
	errChannel := o.instance.errs
	go func() {
		if err := cmd.Wait(); err != nil {
			errChannel <- errors.Wrap(err, "osqueryd processes died")
		} else {
			// Close the channel so that the goroutine blocked
			// waiting for errors will be able to exit
			close(errChannel)
		}
	}()

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
		o.instance.extensionManagerServer, err = osquery.NewExtensionManagerServer(
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
			return nil, errors.Wrapf(err, "could not create extension manager server at %s", paths.extensionSocketPath)
		}
	}

	o.instance.extensionManagerClient, err = osquery.NewClient(paths.extensionSocketPath, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "could not create an extension client")
	}

	plugins := o.instance.extensionPlugins
	for _, t := range PlatformTables(o.instance.extensionManagerClient, o.logger) {
		plugins = append(plugins, t)
	}
	o.instance.extensionManagerServer.RegisterPlugin(plugins...)

	// Launch the extension manager server asynchronously.
	go func() {
		if err := o.instance.extensionManagerServer.Start(); err != nil {
			errChannel <- errors.Wrap(err, "the extension server died")
		}
	}()

	// Briefly sleep so that osqueryd has time to register all extensions
	time.Sleep(2 * time.Second)

	// Launch a long-running recovery goroutine which can handle various errors
	// that can occur
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			needsRecovery := false
			select {
			case <-ticker.C:
				healthy, err := o.Healthy()
				if err != nil {
					needsRecovery = true
					level.Error(o.logger).Log("err", errors.Wrap(err, "checking instance health"))
				}
				if !healthy {
					needsRecovery = true
					level.Error(o.logger).Log("msg", "instance not healthy")
				}

			// Block until an error is generated by the osqueryd process itself or the
			// extension manager server. We don't select, because if one element of the
			// runtime produces an error, it's likely that all of the other components
			// will produce errors as well since everything is so interconnected. For
			// this reason, when any error occurs, we attempt a total recovery.
			case runtimeError, open := <-errChannel:
				if !open {
					return
				}
				needsRecovery = true
				level.Error(o.logger).Log("err", errors.Wrap(runtimeError, "osquery runtime error"))
			}

			o.instanceLock.Lock()
			teardownStarted := o.teardownStarted()
			o.instanceLock.Unlock()
			if needsRecovery && !teardownStarted {
				level.Info(o.logger).Log("msg", "recovering osquery instance")
				if recoveryError := o.Recover(); recoveryError != nil {
					// If we were not able to recover the osqueryd process for some reason,
					// kill the process and hope that the operating system scheduling
					// mechanism (launchd, etc) can relaunch the tool cleanly.
					level.Error(o.logger).Log("err", errors.Wrap(recoveryError, "could not recover the osqueryd process"))
					os.Exit(1)
				}
				return
			}
		}
	}()

	return o, nil

}

// Helper to check whether teardown should commence. This will atomically set
// the teardown flag, and return true if teardown should commence, or false if
// teardown has already begun.
func (o *OsqueryInstance) beginTeardown() bool {
	begun := atomic.SwapInt32(&o.instance.hasBegunTeardown, 1)
	return begun == 0
}

func (o *OsqueryInstance) teardownStarted() bool {
	begun := atomic.LoadInt32(&o.instance.hasBegunTeardown)
	return begun != 0
}

// Recover attempts to launch a new osquery instance if the running instance has
// failed for some reason. Note that this function does not call o.Kill() to
// release resources because Kill() expects the osquery instance to be healthy,
// whereas Recover() expects a hostile environment and is slightly more
// defensive in it's actions.
func (o *OsqueryInstance) Recover() error {
	o.instanceLock.Lock()
	defer o.instanceLock.Unlock()
	// If the user explicitly calls o.Kill(), as the components are shutdown, they
	// may exit with errors. In this case, we shouldn't recover the
	// instance.
	if !o.beginTeardown() {
		return nil
	}

	// First, we try to kill the osqueryd process if it isn't already dead.
	if o.instance.cmd.Process != nil {
		if err := o.instance.cmd.Process.Kill(); err != nil {
			if !strings.Contains(err.Error(), "process already finished") {
				return errors.Wrap(err, "could not kill the osquery process during recovery")
			}
		}
	}

	// Next, we try to kill the osquery extension manager server if it isn't
	// already dead.
	status, err := o.instance.extensionManagerServer.Ping()
	if err == nil && status.Code == int32(0) {
		if err := o.instance.extensionManagerServer.Shutdown(); err != nil {
			return errors.Wrap(err, "could not kill the extension manager server")
		}
	}

	return o.relaunchAndReplace()
}

// Kill will terminate all osquery processes managed by the OsqueryInstance
// instance and release all resources that have been acquired throughout the
// process lifecycle.
func (o *OsqueryInstance) Kill() error {
	if !o.beginTeardown() {
		return errors.New("Will not kill osqueryd instance because teardown has already begun somewhere else")
	}

	// if ok, err := o.Healthy(); err != nil {
	// 	return errors.Wrap(err, "an error occured trying to determine osquery's health")
	// } else if !ok {
	// 	return errors.Wrap(err, "osquery is not healthy")
	// }

	if err := o.instance.cmd.Process.Kill(); err != nil {
		return errors.Wrap(err, "could not kill the osqueryd process")
	}

	if err := o.instance.extensionManagerServer.Shutdown(); err != nil {
		return errors.Wrap(err, "could not kill the extension manager server")
	}

	o.instance.rmRootDirectory()

	return nil
}

// Restart allows you to cleanly shutdown the current osquery instance and launch
// a new osquery instance with the same configurations.
func (o *OsqueryInstance) Restart() error {
	o.instanceLock.Lock()
	defer o.instanceLock.Unlock()
	if err := o.Kill(); err != nil {
		return errors.Wrap(err, "could not kill the osqueryd instance")
	}

	if err := o.relaunchAndReplace(); err != nil {
		return errors.Wrap(err, "could not relaunch osquery instance")
	}

	return nil
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy.
func (o *OsqueryInstance) Healthy() (bool, error) {
	o.instanceLock.Lock()
	defer o.instanceLock.Unlock()
	serverStatus, err := o.instance.extensionManagerServer.Ping()
	if err != nil {
		return false, errors.Wrap(err, "could not ping extension server")
	}

	clientStatus, err := o.instance.extensionManagerClient.Ping()
	if err != nil {
		return false, errors.Wrap(err, "could not ping osquery extension client")
	}
	return serverStatus.Code == 0 && clientStatus.Code == 0, nil
}

func (o *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	o.instance.clientLock.Lock()
	defer o.instance.clientLock.Unlock()
	resp, err := o.instance.extensionManagerClient.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "could not query the extension manager client")
	}
	if resp.Status.Code != int32(0) {
		return nil, errors.New(resp.Status.Message)
	}

	return resp.Response, nil
}

// relaunchAndReplace is an internal helper for launching a new osquery
// instance with the same options as the existing instance and replacing the
// internal o.osqueryInstanceFields with the new instance.
func (o *OsqueryInstance) relaunchAndReplace() error {
	// In order to be sure we are launching a new osquery instance with the same
	// configuration, we define a set of OsqueryInstanceOptions based on the
	// configuration of the previous OsqueryInstance.
	opts := []OsqueryInstanceOption{
		WithOsquerydBinary(o.instance.binaryPath),
		WithConfigPluginFlag(o.instance.configPluginFlag),
		WithLoggerPluginFlag(o.instance.loggerPluginFlag),
		WithDistributedPluginFlag(o.instance.distributedPluginFlag),
		WithRetries(o.instance.retries),
	}
	if !o.instance.usingTempDir {
		opts = append(opts, WithRootDirectory(o.instance.rootDirectory))
	}
	for _, plugin := range o.instance.extensionPlugins {
		opts = append(opts, WithOsqueryExtensionPlugin(plugin))
	}
	newInstance, err := LaunchOsqueryInstance(opts...)
	if err != nil {
		return errors.Wrap(err, "could not launch new osquery instance")
	}

	// Now that we have a new running osquery instance, we replace the fields of
	// our old instance with a pointer to the fields of the new instance so that
	// existing references still work properly.
	o.instance = newInstance.instance

	return nil
}
