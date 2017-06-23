package osquery

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/osquery/table"
	"github.com/kolide/osquery-go"
	"github.com/pkg/errors"
)

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	*osqueryInstanceFields
	*osqueryInstanceOptions
}

type osqueryInstanceOptions struct {
	binaryPath    string
	rootDirectory string
	configPlugin  string
	loggerPlugin  string
	plugins       []osquery.OsqueryPlugin
}

// osqueryInstanceFields is a type which is embedded in OsqueryInstance so that
// in the event that the underlying process instance changes, the fields can be
// updated wholesale without updating the actual OsqueryInstance pointer which
// may be held by the original caller.
type osqueryInstanceFields struct {
	cmd                    *exec.Cmd
	errs                   chan error
	extensionManagerServer *osquery.ExtensionManagerServer
	paths                  *osqueryFilePaths
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

// calculateOsqueryPaths accepts a the path to a working osqueryd binary and a
// root directory where all of the osquery filesystem artifacts should be stored.
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

func WithOsqueryExtensionPlugin(plugin osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.plugins = append(i.plugins, plugin)
	}
}

func WithOsquerydBinary(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.binaryPath = path
	}
}

func WithRootDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.rootDirectory = path
	}
}

func WithConfigPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.configPlugin = plugin
	}
}

func WithLoggerPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.loggerPlugin = plugin
	}
}

// LaunchOsqueryInstance will launch an osqueryd binary. The binaryPath parameter
// should be a valid path to an osqueryd binary. The rootDir parameter should be a
// valid directory where the osquery database and pidfile can be stored. If any
// errors occur during process initialization, an error will be returned.
func LaunchOsqueryInstance(opts ...OsqueryInstanceOption) (*OsqueryInstance, error) {
	o := &OsqueryInstance{
		&osqueryInstanceFields{
			errs: make(chan error),
		},
		&osqueryInstanceOptions{},
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

	if o.configPlugin == "" {
		// TODO
	}

	if o.loggerPlugin == "" {
		// TODO
	}

	cmd, err := createOsquerydCommand(o.binaryPath, paths, o.configPlugin, o.loggerPlugin)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create osqueryd command")
	}
	o.cmd = cmd

	if err := o.cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "could not start the osqueryd command")
	}

	// Launch a long running goroutine to which will keep tabs on the health of
	// the osqueryd process
	go func() {
		if err := o.cmd.Wait(); err != nil {
			o.errs <- errors.Wrap(err, "osqueryd processes died")
		}
	}()

	// If the caller has indicated that the osquery instance should launch one or
	// more plugins, we need to launch an extension server
	if len(o.plugins) > 0 {
		// Briefly sleep so that osqueryd has time to initialize before starting the
		// extension manager server
		time.Sleep(2 * time.Second)

		// Create the extension server
		extensionServer, err := osquery.NewExtensionManagerServer("kolide", paths.extensionSocketPath)
		if err != nil {
			return nil, errors.Wrapf(err, "could not create extension manager server at %s", paths.extensionSocketPath)
		}
		o.extensionManagerServer = extensionServer

		// Register all custom osquery plugins
		extensionServer.RegisterPlugin(o.plugins...)

		// register all platform specific table plugins
		for _, t := range table.PlatformTables() {
			extensionServer.RegisterPlugin(t)
		}

		// Launch the server asynchronously
		go func() {
			if err := extensionServer.Start(); err != nil {
				o.errs <- errors.Wrap(err, "the extension server died")
			}
		}()

		// Launch a long-running recovery goroutine which can handle various errors
		// that can occur
		go func() {
			<-o.errs
			if recoveryError := o.Recover(); recoveryError != nil {
				log.Fatalf("Could not recover the osqueryd process: %s\n", recoveryError)
			}
		}()
	}

	return o, nil
}

// Recover attempts to launch a new osquery instance if the running instance has
// failed for some reason.
func (o *OsqueryInstance) Recover() error {
	if !o.cmd.ProcessState.Exited() {
		if err := o.extensionManagerServer.Shutdown(); err != nil {
			return errors.Wrap(err, "could not shutdown the osquery extension")
		}
		if err := o.cmd.Process.Kill(); err != nil {
			return errors.Wrap(err, "could not kill the osquery process during recovery")
		}
	}

	opts := []OsqueryInstanceOption{
		WithOsquerydBinary(o.binaryPath),
		WithRootDirectory(o.rootDirectory),
		WithConfigPluginFlag(o.configPlugin),
		WithLoggerPluginFlag(o.loggerPlugin),
	}
	for _, plugin := range o.plugins {
		opts = append(opts, WithOsqueryExtensionPlugin(plugin))
	}
	newInstance, err := LaunchOsqueryInstance(
		opts...,
	)
	if err != nil {
		return errors.Wrap(err, "could not launch new osquery instance")
	}

	o.osqueryInstanceFields = newInstance.osqueryInstanceFields
	o.osqueryInstanceOptions = newInstance.osqueryInstanceOptions

	return nil
}

// Kill will terminate all managed osquery processes and release all resources.
func (o *OsqueryInstance) Kill() error {
	if ok, err := o.Healthy(); err != nil {
		return errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return errors.Wrap(err, "osquery is not healthy")
	}

	if err := o.cmd.Process.Kill(); err != nil {
		return errors.Wrap(err, "could not find the watcher process")
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
