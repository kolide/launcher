package osquery

import (
	"os"

	"github.com/pkg/errors"
)

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	pid int
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
			return nil, errors.Wrapf(err, "could not stat supplied osquery instance patch")
		}
	}

	// TODO Launch the osqueryd process

	// TODO Launch a goroutine to continuously check if osquery has died

	oi := &OsqueryInstance{}
	// TODO populate OsqueryInstance with relevant data

	if ok, err := oi.Healthy(); !ok {
		return nil, errors.Wrap(err, "osquery instance instantly became unhealthy after launch")
	}

	return oi, nil
}

// Kill will terminate all managed osquery processes and release all resources.
func (o *OsqueryInstance) Kill() error {
	if ok, err := o.Healthy(); !ok {
		return errors.Wrap(err, "could not kill osquery because it was not healthy")
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
	if ok, err := o.Healthy(); !ok {
		return 0, errors.Wrap(err, "could not get the pid because osquery was not healthy")
	}

	return o.pid, nil
}
