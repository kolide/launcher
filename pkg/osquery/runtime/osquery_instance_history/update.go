package osquery_instance_history

import "sync"

var mutex sync.Mutex
var currentHistory = &History{}

// InstanceStarted adds a new instance to the osquery instance history
func InstanceStarted() error {
	mutex.Lock()
	defer mutex.Unlock()

	return currentHistory.nextInstance()
}

// InstanceConnected sets the connect time and instance id of the current osquery instance
func InstanceConnected(querier Querier) error {
	mutex.Lock()
	defer mutex.Unlock()

	err := currentHistory.currentInstance().connected(querier)
	if err != nil {
		currentHistory.currentInstance().addError(err)
	}

	return err
}

// InstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func InstanceExited(err error) {
	mutex.Lock()
	defer mutex.Unlock()

	currentHistory.currentInstance().exited(err)
}
