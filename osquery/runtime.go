package osquery

import (
	"log"
	"os"
	"time"

	"github.com/kolide/osquery-go"
	"github.com/pkg/errors"
)

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	pid                 int
	extensionSocketPath string
}

// LaunchOsqueryInstance will launch an osqueryd binary. The path parameter
// should be a valid path to an osqueryd binary. If any errors occur during
// process initialization, an error will be returned.
func LaunchOsqueryInstance(path string) (*OsqueryInstance, error) {
	// Ensure that the supplied path exists
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "supplied osquery instance path: %s", path)
		} else {
			return nil, errors.Wrapf(err, "could not stat supplied osquery instance path")
		}
	}

	// TODO Launch the osqueryd process

	// TODO Get the socket path from the extension proxy
	extensionSocketPath := "/not/quite/sure/yet"

	// Create the extension server
	extensionServer, err := osquery.NewExtensionManagerServer("kolide_agent", extensionSocketPath, 1*time.Second)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create extension manager server at %s", extensionSocketPath)
	}

	// Register all custom osquery plugins
	extensionServer.RegisterPlugin(osquery.NewLoggerPlugin(&ExampleLogger{}))
	extensionServer.RegisterPlugin(osquery.NewConfigPlugin(&ExampleConfig{}))
	extensionServer.RegisterPlugin(osquery.NewTablePlugin(&ExampleTable{}))

	// Launch the server asynchronously
	go func() {
		if err := extensionServer.Start(); err != nil {
			log.Println(errors.Wrap(err, "starting/running the extension server"))
			// TODO Relaunch extension server / try to recover
		}
	}()

	// Create the reference instance for the running osquery instance
	o := &OsqueryInstance{
		extensionSocketPath: extensionSocketPath,
	}

	// Ensure that the recently created instance is healthy
	if ok, err := o.Healthy(); err != nil {
		return nil, errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return nil, errors.Wrap(err, "osquery is not healthy")
	}

	return o, nil
}

// Kill will terminate all managed osquery processes and release all resources.
func (o *OsqueryInstance) Kill() error {
	if ok, err := o.Healthy(); err != nil {
		return errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return errors.Wrap(err, "osquery is not healthy")
	}

	watcher, err := os.FindProcess(o.pid)
	if err != nil {
		return errors.Wrap(err, "could not find the watcher process")
	}

	return watcher.Kill()
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy.
func (o *OsqueryInstance) Healthy() (bool, error) {
	// TODO Query the osquery_info table and update the OsqueryInstance data
	// structure if any information has changed

	return false, errors.New("not implemented")
}

// Pid returns the process ID of the osqueryd watcher process (or whatever the
// most senior process ID is). If the osquery instance is not healthy, an error
// will be returned.
func (o *OsqueryInstance) Pid() (int, error) {
	if ok, err := o.Healthy(); err != nil {
		return 0, errors.Wrap(err, "an error occured trying to determine osquery's health")
	} else if !ok {
		return 0, errors.Wrap(err, "osquery is not healthy")
	}

	return o.pid, nil
}
