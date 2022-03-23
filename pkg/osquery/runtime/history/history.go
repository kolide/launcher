package history

import (
	"errors"
	"os"
	"sync"
	"time"
)

const maxInstances = 10

var currentHistory *History
var currentHistoryMutext sync.Mutex

type History struct {
	mutex     sync.Mutex
	instances []*Instance
}

type NoCurrentHistoryError struct{}

func (c NoCurrentHistoryError) Error() string {
	return "no history has been created"
}

type NoCurrentInstanceError struct{}

func (c NoCurrentInstanceError) Error() string {
	return "no osquery instance is currently set"
}

type CurrentInstanceNotExitedError struct{}

func (c CurrentInstanceNotExitedError) Error() string {
	return "cannot create new instance of osquery history while current instance does not have exit time"
}

type HistoryAlreadyCreatedError struct{}

func (c HistoryAlreadyCreatedError) Error() string {
	return "history has already been created, access readonly data via package functions"
}

// NewHistory creates a new history and sets it as the current history.
// If the current history already been set, gives an error.
func NewHistory() (*History, error) {
	currentHistoryMutext.Lock()
	defer currentHistoryMutext.Unlock()

	if currentHistory != nil {
		return nil, HistoryAlreadyCreatedError{}
	}

	currentHistory = &History{}
	return currentHistory, nil
}

// NewInstanceStarted adds a new instance to the osquery instance history
func (h *History) NewInstanceStarted() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.newInstanceStarted()
}

// CurrentInstanceConnected sets the connect time and instance id of the current osquery instance
func (h *History) CurrentInstanceConnected(querier Querier) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.currentInstanceConnected(querier)
}

// CurrentInstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func (h *History) CurrentInstanceExited(exitError error) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.currentInstanceExited(exitError)
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher, each start / restart cycle is an entry
func GetHistory() ([]Instance, error) {
	if currentHistory == nil {
		return nil, NoCurrentHistoryError{}
	}

	currentHistory.mutex.Lock()
	defer currentHistory.mutex.Unlock()

	results := make([]Instance, len(currentHistory.instances))
	for i, v := range currentHistory.instances {
		results[i] = *v
	}

	return results, nil
}

// CurrentInstance returns the current osquery instance
func CurrentInstance() (Instance, error) {
	if currentHistory == nil {
		return Instance{}, NoCurrentHistoryError{}
	}

	currentHistory.mutex.Lock()
	defer currentHistory.mutex.Unlock()

	instance, err := currentHistory.currentInstance()
	if err != nil {
		return Instance{}, err
	}

	return *instance, nil
}

func (h *History) currentInstance() (*Instance, error) {
	if h.instances != nil && len(h.instances) > 0 {
		return h.instances[len(h.instances)-1], nil
	}
	return nil, NoCurrentInstanceError{}
}

func (h *History) newInstanceStarted() error {
	currentInstance, err := h.currentInstance()
	if err != nil && !errors.Is(err, NoCurrentInstanceError{}) {
		return err
	}

	if currentInstance != nil && currentInstance.ExitTime == "" {
		return CurrentInstanceNotExitedError{}
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	newInstance := &Instance{
		StartTime: timeNow(),
		Hostname:  hostname,
	}

	h.addCurrentInstance(newInstance)

	return nil
}

func (h *History) addCurrentInstance(instance *Instance) {
	if h.instances == nil {
		h.instances = []*Instance{instance}
		return
	}

	h.instances = append(h.instances, instance)

	if len(h.instances) >= maxInstances {
		h.instances = h.instances[len(h.instances)-maxInstances:]
	}
}

func (h *History) currentInstanceConnected(querier Querier) error {
	currentInstance, err := h.currentInstance()
	if err != nil {
		return err
	}

	err = currentInstance.connected(querier)
	if err != nil {
		currentInstance.addError(err)
	}

	return err
}

// InstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func (h *History) currentInstanceExited(exitError error) error {
	currentInstance, err := h.currentInstance()
	if err != nil {
		return err
	}

	currentInstance.exited(exitError)
	return nil
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
