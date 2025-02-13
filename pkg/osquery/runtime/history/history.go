package history

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

const maxInstances = 10

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
func InitHistory(store types.GetterSetter) (*History, error) {
	history := History{
		instances: make([]*Instance, 0),
		store:     store,
	}

	if err := history.load(); err != nil {
		return nil, fmt.Errorf("error loading osquery_instance_history: %w", err)
	}

	return &history, nil
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher, each start / restart cycle is an entry
func (h *History) GetHistory() ([]map[string]string, error) {
	h.Lock()
	defer h.Unlock()

	if len(h.instances) == 0 {
		return nil, NoInstancesError{}
	}

	results := make([]map[string]string, len(h.instances))
	for i, v := range h.instances {
		results[i] = v.toMap()
	}

	return results, nil
}

func (h *History) latestInstance(registrationId string) (*Instance, error) {
	if len(h.instances) == 0 {
		return nil, NoInstancesError{}
	}

	for i := len(h.instances) - 1; i > -1; i -= 1 {
		if h.instances[i].RegistrationId == registrationId {
			return h.instances[i], nil
		}
	}

	return nil, NoInstancesError{}
}

func (h *History) LatestInstanceStats(registrationId string) (map[string]string, error) {
	h.Lock()
	defer h.Unlock()

	instance, err := h.latestInstance(registrationId)
	if err != nil {
		return nil, err
	}

	return instance.toMap(), nil
}

func (h *History) LatestInstanceId(registrationId string) (string, error) {
	h.Lock()
	defer h.Unlock()

	instance, err := h.latestInstance(registrationId)
	if err != nil {
		return "", err
	}

	return instance.InstanceId, nil
}

func (h *History) LatestInstanceUptimeMinutes(registrationId string) (int64, error) {
	lastInstance, err := h.latestInstance(registrationId)
	if err != nil {
		return 0, fmt.Errorf("getting latest instance: %w", err)
	}

	if lastInstance.ExitTime != "" {
		return 0, nil
	}

	startTime, err := time.Parse(time.RFC3339, lastInstance.StartTime)
	if err != nil {
		return 0, fmt.Errorf("parsing start time %s: %w", lastInstance.StartTime, err)
	}

	uptimeSeconds := time.Now().UTC().Unix() - startTime.Unix()
	return uptimeSeconds / 60, nil
}

// NewInstance adds a new instance to the osquery instance history and returns it
func (h *History) NewInstance(registrationId string, runId string) error {
	h.Lock()
	defer h.Unlock()

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	newInstance := &Instance{
		RegistrationId: registrationId,
		RunId:          runId,
		StartTime:      timeNow(),
		Hostname:       hostname,
	}

	h.addInstanceToHistory(newInstance)

	if err := h.save(); err != nil {
		return fmt.Errorf("error saving osquery_instance_history: %w", err)
	}

	return nil
}

func (h *History) SetConnected(runID string, querier types.Querier) error {
	h.Lock()
	defer h.Unlock()

	instanceFound := false
	for i := len(h.instances) - 1; i > -1; i -= 1 {
		if h.instances[i].RunId != runID {
			continue
		}

		instanceFound = true
		if err := h.instances[i].Connected(querier); err != nil {
			return fmt.Errorf("error setting connected for osquery instance: %w", err)
		}
	}

	if !instanceFound {
		return NoInstancesError{}
	}

	if err := h.save(); err != nil {
		return fmt.Errorf("error saving osquery_instance_history: %w", err)
	}

	return nil
}

func (h *History) SetExited(runID string, exitError error) error {
	h.Lock()
	defer h.Unlock()

	instanceFound := false
	for i := len(h.instances) - 1; i > -1; i -= 1 {
		if h.instances[i].RunId != runID {
			continue
		}

		instanceFound = true
		if err := h.instances[i].Exited(exitError); err != nil {
			return fmt.Errorf("error setting exited for osquery instance: %w", err)
		}
	}

	if !instanceFound {
		return NoInstancesError{}
	}

	if err := h.save(); err != nil {
		return fmt.Errorf("error saving osquery_instance_history: %w", err)
	}

	return nil
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
