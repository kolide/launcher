package history

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
)

const maxInstances = 10

var currentHistory *History = &History{}

type History struct {
	sync.Mutex
	instances []*Instance
	store     types.GetterSetter
}

type NoInstancesError struct{}

func (c NoInstancesError) Error() string {
	return "no osquery instances have been added to history"
}

// InitHistory loads the osquery instance history from bbolt DB if exists, sets up bucket if it does not
func InitHistory(store types.GetterSetter) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	currentHistory.store = store

	if err := currentHistory.load(); err != nil {
		return fmt.Errorf("error loading osquery_instance_history: %w", err)
	}

	return nil
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher, each start / restart cycle is an entry
func GetHistory() ([]Instance, error) {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	if currentHistory.instances == nil {
		return nil, NoInstancesError{}
	}

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

	if currentHistory.instances == nil || len(currentHistory.instances) == 0 {
		return Instance{}, NoInstancesError{}
	}

	return *currentHistory.instances[len(currentHistory.instances)-1], nil
}

// NewInstance adds a new instance to the osquery instance history and returns it
func NewInstance() (*Instance, error) {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	newInstance := &Instance{
		StartTime: timeNow(),
		Hostname:  hostname,
	}

	currentHistory.addInstanceToHistory(newInstance)

	if err := currentHistory.save(); err != nil {
		return newInstance, fmt.Errorf("error saving osquery_instance_history: %w", err)
	}

	return newInstance, nil
}

func (h *History) addInstanceToHistory(instance *Instance) {
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
