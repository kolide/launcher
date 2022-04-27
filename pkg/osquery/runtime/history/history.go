package history

import (
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const maxInstances = 10

var currentHistory *History = &History{}

type History struct {
	sync.Mutex
	instances []*Instance
	db        *bbolt.DB
}

type NoInstancesError struct{}

func (c NoInstancesError) Error() string {
	return "no osquery instances have been added to history"
}

// InitHistory loads the osquery instance history from bbolt DB if exists, sets up bucket if it does not
func InitHistory(db *bbolt.DB) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	err := createBboltBucketIfNotExists(db)
	if err != nil {
		return err
	}

	currentHistory.db = db

	err = currentHistory.load()

	if err != nil {
		return errors.Wrap(err, "error loading osquery_instance_history")
	}

	return nil
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

	err = currentHistory.save()

	if err != nil {
		return newInstance, errors.Wrap(err, "error saving osquery_instance_history")
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
