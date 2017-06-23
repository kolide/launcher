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
	plugins                []osquery.OsqueryPlugin
	configPlugin           string
	loggerPlugin           string
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	RootDir               string
	BinaryPath            string
	PidfilePath           string
	DatabasePath          string
	ExtensionPath         string
	ExtensionAutoloadPath string
	ExtensionSocketPath   string
}

// calculateOsqueryPaths accepts a the path to a working osqueryd binary and a
// root directory where all of the osquery filesystem artifacts should be stored.
// In return, a structure of paths is returned that can be used to launch an
// osqueryd instance. An error may be returned if the supplied parameters are
// unacceptable.
func calculateOsqueryPaths(binaryPath, rootDir string) (*osqueryFilePaths, error) {
	// Ensure that the supplied path exists
	if _, err := os.Stat(binaryPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "osquery instance path does not exist: %s", binaryPath)
		} else {
			return nil, errors.Wrapf(err, "could not stat supplied osquery instance path")
		}
	}

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
		RootDir:               rootDir,
		BinaryPath:            binaryPath,
		PidfilePath:           filepath.Join(rootDir, "osquery.pid"),
		DatabasePath:          filepath.Join(rootDir, "osquery.db"),
		ExtensionPath:         extensionPath,
		ExtensionAutoloadPath: extensionAutoloadPath,
		ExtensionSocketPath:   filepath.Join(rootDir, "osquery.sock"),
	}, nil
}

// createOsquerydCommand accepts a structure of relevant file paths relating to
// an osquery instance and returns an *exec.Cmd which will launch a properly
// configured osqueryd process.
func createOsquerydCommand(paths *osqueryFilePaths, configPlugin, loggerPlugin string) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	cmd := exec.Command(
		paths.BinaryPath,
		fmt.Sprintf("--pidfile=%s", paths.PidfilePath),
		fmt.Sprintf("--database_path=%s", paths.DatabasePath),
		fmt.Sprintf("--extensions_socket=%s", paths.ExtensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", paths.ExtensionAutoloadPath),
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

type OsqueryInstanceOption func(*OsqueryInstance)

func WithPlugin(plugin osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.plugins = append(i.plugins, plugin)
	}
}

// LaunchOsqueryInstance will launch an osqueryd binary. The binaryPath parameter
// should be a valid path to an osqueryd binary. The rootDir parameter should be a
// valid directory where the osquery database and pidfile can be stored. If any
// errors occur during process initialization, an error will be returned.
func LaunchOsqueryInstance(binaryPath, rootDir, configPlugin, loggerPlugin string, opts ...OsqueryInstanceOption) (*OsqueryInstance, error) {
	paths, err := calculateOsqueryPaths(binaryPath, rootDir)
	if err != nil {
		return nil, errors.Wrap(err, "could not calculate osquery file paths")
	}

	cmd, err := createOsquerydCommand(paths, configPlugin, loggerPlugin)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create osqueryd command")
	}

	o := &OsqueryInstance{
		&osqueryInstanceFields{
			cmd:          cmd,
			errs:         make(chan error),
			paths:        paths,
			configPlugin: configPlugin,
			loggerPlugin: loggerPlugin,
		},
	}
	for _, opt := range opts {
		opt(o)
	}

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

	// Briefly sleep so that osqueryd has time to initialize before starting the
	// extension manager server
	time.Sleep(2 * time.Second)

	// Create the extension server
	extensionServer, err := osquery.NewExtensionManagerServer("kolide", paths.ExtensionSocketPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create extension manager server at %s", paths.ExtensionSocketPath)
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

	newInstance, err := LaunchOsqueryInstance(o.paths.BinaryPath, o.paths.RootDir, o.configPlugin, o.loggerPlugin)
	if err != nil {
		return errors.Wrap(err, "could not launch new osquery instance")
	}

	o.osqueryInstanceFields = newInstance.osqueryInstanceFields

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
