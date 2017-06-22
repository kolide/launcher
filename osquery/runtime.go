package osquery

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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
	cmd                    *exec.Cmd
	errs                   chan error
	binaryPath             string
	rootDir                string
	extensionSocketPath    string
	extensionManagerServer *osquery.ExtensionManagerServer
}

// LaunchOsqueryInstance will launch an osqueryd binary. The binaryPath parameter
// should be a valid path to an osqueryd binary. The rootDir parameter should be a
// valid directory where the osquery database and pidfile can be stored. If any
// errors occur during process initialization, an error will be returned.
func LaunchOsqueryInstance(binaryPath string, rootDir string) (*OsqueryInstance, error) {
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

	// Create the reference instance for the running osquery instance
	pidfilePath := filepath.Join(rootDir, "osquery.pid")
	databasePath := filepath.Join(rootDir, "osquery.db")
	extensionSocketPath := filepath.Join(rootDir, "osquery.sock")

	cmd := exec.Command(
		binaryPath,
		fmt.Sprintf("--pidfile=%s", pidfilePath),
		fmt.Sprintf("--database_path=%s", databasePath),
		fmt.Sprintf("--extensions_socket=%s", extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", extensionAutoloadPath),
		"--pack_delimiter=:",
		"--config_refresh=10",
		"--config_plugin=kolide_grpc",
		"--logger_plugin=kolide_grpc",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	o := &OsqueryInstance{
		&osqueryInstanceFields{
			cmd:                 cmd,
			errs:                make(chan error),
			binaryPath:          binaryPath,
			rootDir:             rootDir,
			extensionSocketPath: extensionSocketPath,
		},
	}

	if err := o.cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "could not start the osqueryd command")
	}

	// Launch a long running goroutine to which will keep tabs on the health of
	// the osqueryd process
	go func() {
		if err := o.cmd.Wait(); err != nil {
			o.errs <- errors.Wrap(err, "osqueryd processes died due to an error")
		} else {
			o.errs <- errors.New("osqueryd processes exited successfully")
		}
	}()

	// Briefly sleep so that osqueryd has time to initialize before starting the
	// extension manager server
	time.Sleep(2 * time.Second)

	// Create the extension server
	extensionServer, err := osquery.NewExtensionManagerServer("kolide", extensionSocketPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create extension manager server at %s", extensionSocketPath)
	}
	o.extensionManagerServer = extensionServer

	// Register all custom osquery plugins
	extensionServer.RegisterPlugin(
		config.NewPlugin("kolide_grpc", GenerateConfigs),
		logger.NewPlugin("kolide_grpc", LogString),
	)

	// Launch the server asynchronously
	go func() {
		if err := extensionServer.Start(); err != nil {
			o.errs <- errors.Wrap(err, "the extension server died due to an error")
		} else {
			o.errs <- errors.New("the extension server exited prematurely")
		}
	}()

	// Ensure that the recently created instance is healthy
	if ok, err := o.Healthy(); err != nil {
		return nil, errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return nil, errors.Wrap(err, "osquery is not healthy")
	}

	// Launch a long-running recovery goroutine which can handle various errors
	// that can occur
	go func() {
		select {
		case executionError := <-o.errs:
			if recoveryError := o.Recover(executionError); recoveryError != nil {
				log.Fatalf("Could not recover the osqueryd process: %s\n", recoveryError)
			} else {
				log.Printf("Received an execution error, but successfully recovered from it: %s\n", executionError)
			}
		}
	}()

	return o, nil
}

// Recover attempts to launch a new osquery instance if the running instance has
// failed for some reason. executionError is the error that occurred to the
// instance of osqueryd that has caused it to stop running.
func (o *OsqueryInstance) Recover(executionError error) error {
	if !o.cmd.ProcessState.Exited() {
		if err := o.cmd.Process.Kill(); err != nil {
			return errors.Wrap(err, "could not kill the osquery process during recovery")
		}
	}

	newInstance, err := LaunchOsqueryInstance(o.binaryPath, o.rootDir)
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
