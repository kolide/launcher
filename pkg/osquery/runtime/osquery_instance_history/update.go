package osquery_instance_history

import "sync"

var mutex sync.Mutex
var currentHistory = &History{}

// Started adds a new instance to the osquery instance history
func Started() error {
	mutex.Lock()
	defer mutex.Unlock()

	return currentHistory.nextInstance()
}

// Connected sets the start time and instance id of the current osquery instance
func Connected(querier Querier) error {
	mutex.Lock()
	defer mutex.Unlock()

	err := currentHistory.currentInstance().connected(querier)
	if err != nil {
		currentHistory.currentInstance().addError(err)
	}

	return err
}

// Exited sets the exit time and appends provided error (if any) to current osquery instance
func Exited(err error) {
	mutex.Lock()
	defer mutex.Unlock()

	currentHistory.currentInstance().exited(err)
}
