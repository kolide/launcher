package osquery_instance_history

import (
	"errors"
	"sync"
	"time"
)

const maxInstances = 10

var currentHistory = &history{}

type history struct {
	sync.Mutex
	instances []*Instance
}

type NoCurrentInstanceError struct{}

func (c NoCurrentInstanceError) Error() string {
	return "no osquery instance is currently set"
}

// InstanceStarted adds a new instance to the osquery instance history
func InstanceStarted() error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	return currentHistory.nextInstance()
}

// InstanceConnected sets the connect time and instance id of the current osquery instance
func InstanceConnected(querier Querier) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	return currentHistory.setCurrentInstanceConnected(querier)
}

// InstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func InstanceExited(exitError error) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	return currentHistory.setCurrentInstanceExited(exitError)
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher, each start / restart cycle is an entry
func GetHistory() []Instance {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	results := make([]Instance, len(currentHistory.instances))
	for i, v := range currentHistory.instances {
		results[i] = *v
	}
	return results
}

// CurrentInstance returns the current osquery instance
func CurrentInstance() (Instance, error) {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	instance, err := currentHistory.currentInstance()
	if err != nil {
		return Instance{}, err
	}

	return *instance, nil
}

func (h *history) currentInstance() (*Instance, error) {
	if h.instances != nil && len(h.instances) > 0 {
		return h.instances[len(h.instances)-1], nil
	}
	return nil, NoCurrentInstanceError{}
}

func (h *history) nextInstance() error {
	currentInstance, err := h.currentInstance()
	if err != nil && !errors.Is(err, NoCurrentInstanceError{}) {
		return err
	}

	if currentInstance != nil && currentInstance.ExitTime == "" {
		return errors.New("cannot create new instance of osquery while current instance does not have exit time")
	}

	newInstance := &Instance{
		StartTime: timeNow(),
	}

	h.addCurrentInstance(newInstance)

	return nil
}

func (h *history) addCurrentInstance(instance *Instance) {
	if h.instances == nil {
		h.instances = []*Instance{instance}
		return
	}

	h.instances = append(h.instances, instance)

	if len(h.instances) >= maxInstances {
		h.instances = h.instances[len(h.instances)-maxInstances:]
	}
}

func (h *history) setCurrentInstanceConnected(querier Querier) error {
	currentInstance, err := currentHistory.currentInstance()
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
func (h *history) setCurrentInstanceExited(exitError error) error {
	currentInstance, err := currentHistory.currentInstance()
	if err != nil {
		return err
	}

	currentInstance.exited(exitError)
	return nil
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
