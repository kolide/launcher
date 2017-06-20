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
	cmd        *exec.Cmd
	errs       chan error
	binaryPath string
	workingDir string
}

// LaunchOsqueryInstance will launch an osqueryd binary. The path parameter
// should be a valid path to an osqueryd binary. The root parameter should be a
// valid directory where the osquery database and pidfile can be stored. If any
// errors occur during process initialization, an error will be returned.
func LaunchOsqueryInstance(path string, root string) (*OsqueryInstance, error) {
	// Ensure that the supplied path exists
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "supplied osquery instance path: %s", path)
		} else {
			return nil, errors.Wrapf(err, "could not stat supplied osquery instance path")
		}
	}

	// Launch the osqueryd process
	pidfilePath := filepath.Join(root, "osquery.pid")
	databasePath := filepath.Join(root, "osquery.db")
	extensionSocketPath := filepath.Join(root, "osquery.sock")

	// Determine the path to the extension
	extensionPath := filepath.Join(filepath.Dir(os.Args[0]), "extproxy.ext")

	// Write the autoload file
	extensionAutoloadPath := filepath.Join(root, "osquery.autoload")
	if err := ioutil.WriteFile(extensionAutoloadPath, []byte(extensionPath), 0644); err != nil {
		return nil, errors.Wrap(err, "could not write osquery extension autoload file")
	}

	flagfileContent := fmt.Sprintf(`
--pidfile=%s
--database_path=%s
--extensions_socket=%s
--extensions_autoload=%s
--config_refresh=10
--config_plugin=kolide_grpc
--logger_plugin=kolide_grpc
`, pidfilePath, databasePath, extensionSocketPath, extensionAutoloadPath)
	flagfilePath := filepath.Join(root, "osquery.flags")
	if err := ioutil.WriteFile(flagfilePath, []byte(flagfileContent), 0644); err != nil {
		return nil, errors.Wrap(err, "could not write osquery flagfile")
	}

	// Create the reference instance for the running osquery instance
	cmd := exec.Command(path, fmt.Sprintf("--flagfile=%s", flagfilePath))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	errs := make(chan error)

	o := &OsqueryInstance{
		cmd:        cmd,
		errs:       errs,
		binaryPath: path,
		workingDir: root,
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

	// Register all custom osquery plugins
	extensionServer.RegisterPlugin(config.NewPlugin("kolide_grpc", GenerateConfigs))
	extensionServer.RegisterPlugin(logger.NewPlugin("kolide_grpc", LogString))

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

	go func() {
		var executionError error
		select {
		case o.errs <- executionError:
			if recoveryError := o.Recover(executionError); recoveryError != nil {
				log.Fatalln("Could not recover the osqueryd process: %s", recoveryError)
			} else {
				log.Println("Received an execution error, but successfully recovered from it: %s", executionError)
			}
		}
	}()

	return o, nil
}

func (o *OsqueryInstance) Recover(executionError error) error {
	// TODO recover the instance
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
	// TODO determine whether or not the instance is healthy
	return true, nil
}
