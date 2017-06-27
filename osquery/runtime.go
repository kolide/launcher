package osquery

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kolide/launcher/osquery/table"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	*osqueryInstanceFields
}

// osqueryInstanceFields is a type which is embedded in OsqueryInstance so that
// in the event that the underlying process instance changes, the fields can be
// updated wholesale without updating the actual OsqueryInstance pointer which
// may be held by the original caller.
type osqueryInstanceFields struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	binaryPath       string
	rootDirectory    string
	configPluginFlag string
	loggerPluginFlag string
	extensionPlugins []osquery.OsqueryPlugin

	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	cmd                    *exec.Cmd
	errs                   chan error
	extensionManagerServer *osquery.ExtensionManagerServer
	paths                  *osqueryFilePaths
	once                   sync.Once
	hasBeganTeardown       bool
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

// calculateOsqueryPaths accepts a ath to a working osqueryd binary and a root
// directory where all of the osquery filesystem artifacts should be stored.
// In return, a structure of paths is returned that can be used to launch an
// osqueryd instance. An error may be returned if the supplied parameters are
// unacceptable.
func calculateOsqueryPaths(rootDir string) (*osqueryFilePaths, error) {
	// Determine the path to the extension
	extensionPath := filepath.Join(filepath.Dir(os.Args[0]), "osquery-extension.ext")
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
func createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths, configPlugin, loggerPlugin string) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	cmd := exec.Command(
		osquerydBinary,
		fmt.Sprintf("--pidfile=%s", paths.pidfilePath),
		fmt.Sprintf("--database_path=%s", paths.databasePath),
		fmt.Sprintf("--extensions_socket=%s", paths.extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", paths.extensionAutoloadPath),
		fmt.Sprintf("--config_plugin=%s", configPlugin),
		fmt.Sprintf("--logger_plugin=%s", loggerPlugin),
		"--pack_delimiter=:",
		"--config_refresh=10",
		"--host_identifier=uuid",
		"--force=true",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd, nil
}

// OsqueryInstanceOption is a functional option pattern for defining how an
// osqueryd instance should be configured. For more information on this pattern,
// see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type OsqueryInstanceOption func(*OsqueryInstance)

// WithOsqueryExtensionPlugin is a functional option which allows the user to
// declare a number of osquery plugins (ie: config plugin, logger plugin, tables,
// etc) which can be loaded when calling LaunchOsqueryInstance. You can load as
// many plugins as you'd like.
func WithOsqueryExtensionPlugin(plugin osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.extensionPlugins = append(i.extensionPlugins, plugin)
	}
}

// WithOsquerydBinary is a functional option which allows the user to define the
// path of the osqueryd binary which will be launched. This should only be called
// once as only one binary will be executed. Defining the path to the osqueryd
// binary is optional. If it is not explicitly defined by the caller, an osqueryd
// binary will be looked for in the current $PATH.
func WithOsquerydBinary(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.binaryPath = path
	}
}

// WithRootDirectory is a functional option which allows the user to define the
// path where filesystem artifacts will be stored. This may include pidfiles,
// RocksDB database files, etc. If this is not defined, a temporary directory
// will be used.
func WithRootDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.rootDirectory = path
	}
}

// WithConfigPluginFlag is a functional option which allows the user to define
// which config plugin osqueryd should use to retrieve the config. If this is not
// defined, it is assumed that no configuration is needed and a no-op config
// will be used. This should only be configured once and cannot be changed once
// osqueryd is running.
func WithConfigPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.configPluginFlag = plugin
	}
}

// WithLoggerPluginFlag is a functional option which allows the user to define
// which logger plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default logger. The
// logger plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithLoggerPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.loggerPluginFlag = plugin
	}
}

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
		&osqueryInstanceFields{
			errs: make(chan error),
		},
	}

	for _, opt := range opts {
		opt(o)
	}

	// If the path of the osqueryd binary wasn't explicitly defined by the caller,
	// try to find it in the path.
	if o.binaryPath == "" {
		path, err := exec.LookPath("osqueryd")
		if err != nil {
			return nil, errors.Wrap(err, "osqueryd not supplied and not found")
		}
		o.binaryPath = path
	}

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if o.rootDirectory == "" {
		o.rootDirectory = os.TempDir()
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files.
	paths, err := calculateOsqueryPaths(o.rootDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "could not calculate osquery file paths")
	}

	// If a config plugin has not been set by the caller, then it is likely that
	// the instance will just be used for executing queries, so we will use a
	// minimal config plugin that basically is a no-op.
	if o.configPluginFlag == "" {
		generateConfigs := func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		}
		o.extensionPlugins = append(o.extensionPlugins, config.NewPlugin("internal_noop", generateConfigs))
		o.configPluginFlag = "internal_noop"
	}

	// If a logger plugin has not been set by the caller, we set a logger plugin
	// that outputs logs to the default application logger.
	if o.loggerPluginFlag == "" {
		logString := func(ctx context.Context, typ logger.LogType, logText string) error {
			log.Printf("%s: %s\n", typ, logText)
			return nil
		}
		o.extensionPlugins = append(o.extensionPlugins, logger.NewPlugin("internal_noop", logString))
		o.loggerPluginFlag = "internal_noop"
	}

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	o.cmd, err = createOsquerydCommand(o.binaryPath, paths, o.configPluginFlag, o.loggerPluginFlag)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create osqueryd command")
	}

	if err := o.cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "couldn't start osqueryd command")
	}

	// Launch a long running goroutine to monitor the osqueryd process.
	go func() {
		if err := o.cmd.Wait(); err != nil {
			o.errs <- errors.Wrap(err, "osqueryd processes died")
		}
	}()

	// Briefly sleep so that osqueryd has time to initialize before starting the
	// extension manager server
	time.Sleep(2 * time.Second)

	// Create the extension server and register all custom osquery plugins
	o.extensionManagerServer, err = osquery.NewExtensionManagerServer(
		"kolide",
		paths.extensionSocketPath,
		osquery.ServerTimeout(2*time.Second),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create extension manager server at %s", paths.extensionSocketPath)
	}
	o.extensionManagerServer.RegisterPlugin(o.extensionPlugins...)
	// register all platform specific table plugins
	for _, t := range table.PlatformTables() {
		o.extensionManagerServer.RegisterPlugin(t)
	}

	// Launch the extension manager server asynchronously.
	go func() {
		if err := o.extensionManagerServer.Start(); err != nil {
			o.errs <- errors.Wrap(err, "the extension server died")
		}
	}()

	// Launch a long-running recovery goroutine which can handle various errors
	// that can occur
	go func() {
		// Block until an error is generated by the osqueryd process itself or the
		// extension manager server. We don't select, because if one element of the
		// runtime produces an error, it's likely that all of the other components
		// will produce errors as well since everything is so interconnected. For
		// this reason, when any error occurs, we attempt a total recovery.
		<-o.errs
		o.once.Do(func() {
			if recoveryError := o.Recover(); recoveryError != nil {
				// If we were not able to recover the osqueryd process for some reason,
				// kill the process and hope that the operating system scheduling
				// mechanism (launchd, etc) can relaunch the tool cleanly.
				log.Fatalf("Could not recover the osqueryd process: %s\n", recoveryError)
			}
		})
	}()

	return o, nil
}

// Recover attempts to launch a new osquery instance if the running instance has
// failed for some reason. Note that this function does not call o.Kill() to
// release resources because Kill() expects the osquery instance to be healthy,
// whereas Recover() expects a hostile environment and is slightly more
// defensive in it's actions.
func (o *OsqueryInstance) Recover() error {
	if o.hasBeganTeardown {
		log.Println("Will not recover osqueryd instance because teardown has already begun somewhere else")
		return nil
	}

	o.hasBeganTeardown = true

	// First, we try to kill the osqueryd process if it isn't already dead.
	if !o.cmd.ProcessState.Exited() {
		if err := o.cmd.Process.Kill(); err != nil {
			if !strings.Contains(err.Error(), "process already finished") {
				return errors.Wrap(err, "could not kill the osquery process during recovery")

			}
		}
	}

	// Next, we try to kill the osquery extension manager server if it isn't
	// already dead.
	if _, err := o.extensionManagerServer.Ping(); err != nil {
		if err := o.extensionManagerServer.Shutdown(); err != nil {
			return errors.Wrap(err, "could not kill the extension manager server")
		}
	}

	// In order to be sure we are launching a new osquery instance with the same
	// configuration, we define a set of OsqueryInstanceOptions based on the
	// configuration of the previous OsqueryInstance.
	opts := []OsqueryInstanceOption{
		WithOsquerydBinary(o.binaryPath),
		WithRootDirectory(o.rootDirectory),
		WithConfigPluginFlag(o.configPluginFlag),
		WithLoggerPluginFlag(o.loggerPluginFlag),
	}
	for _, plugin := range o.extensionPlugins {
		opts = append(opts, WithOsqueryExtensionPlugin(plugin))
	}
	newInstance, err := LaunchOsqueryInstance(opts...)
	if err != nil {
		return errors.Wrap(err, "could not launch new osquery instance")
	}

	// Finally, now that we have a new running osquery instance, we replace the
	// fields of our old instance with a pointer to the fields of the new instance
	// so that existing references still work properly.
	o.osqueryInstanceFields = newInstance.osqueryInstanceFields

	return nil
}

// Kill will terminate all osquery processes managed by the OsqueryInstance
// instance and release all resources that have been acquired throughout the
// process lifecycle.
func (o *OsqueryInstance) Kill() error {
	if o.hasBeganTeardown {
		log.Println("Will not kill osqueryd instance because teardown has already begun somewhere else")
		return nil
	}

	o.hasBeganTeardown = true

	if ok, err := o.Healthy(); err != nil {
		return errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return errors.Wrap(err, "osquery is not healthy")
	}

	if err := o.cmd.Process.Kill(); err != nil {
		return errors.Wrap(err, "could not kill the osqueryd process")
	}

	if err := o.extensionManagerServer.Shutdown(); err != nil {
		return errors.Wrap(err, "could not kill the extension manager server")
	}

	return nil
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy.
func (o *OsqueryInstance) Healthy() (bool, error) {
	status, err := o.extensionManagerServer.Ping()
	if err != nil {
		return false, errors.Wrap(err, "could not ping osquery through extension interface")
	}
	return status.Code == 0, nil
}
