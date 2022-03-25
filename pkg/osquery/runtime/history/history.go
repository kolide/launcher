package history

import (
	"errors"
	"os"
	"sync"
	"time"
)

const maxInstances = 10

var currentHistory *History = &History{}

type History struct {
	sync.Mutex
	instances []*Instance
}

type NoInstancesError struct{}

func (c NoInstancesError) Error() string {
	return "no osquery instance is currently set"
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher, each start / restart cycle is an entry
func GetHistory() ([]Instance, error) {
	if currentHistory.instances == nil {
		return nil, NoInstancesError{}
	}

	currentHistory.Lock()
	defer currentHistory.Unlock()

	results := make([]Instance, len(currentHistory.instances))
	for i, v := range currentHistory.instances {
		results[i] = *v
	}

	return results, nil
}

// LatestInstance returns the latest osquery instance
func LatestInstance() (Instance, error) {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	instance, err := currentHistory.latestInstance()
	if err != nil {
		return Instance{}, err
	}

	return *instance, nil
}

func (h *History) latestInstance() (*Instance, error) {
	if h.instances != nil && len(h.instances) > 0 {
		return h.instances[len(h.instances)-1], nil
	}
	return nil, NoInstancesError{}
}

// NewInstance adds a new instance to the osquery instance history and returns it
func NewInstance() (*Instance, error) {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	return currentHistory.newInstance()
}

func (h *History) newInstance() (*Instance, error) {
	_, err := h.latestInstance()
	if err != nil && !errors.Is(err, NoInstancesError{}) {
		return nil, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	newInstance := &Instance{
		StartTime: timeNow(),
		Hostname:  hostname,
	}

	h.addNewInstance(newInstance)

	return newInstance, nil
}

func (h *History) addNewInstance(instance *Instance) {
	if h.instances == nil {
		h.instances = []*Instance{instance}
		return
	}

	h.instances = append(h.instances, instance)

	if len(h.instances) >= maxInstances {
		h.instances = h.instances[len(h.instances)-maxInstances:]
	}
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
