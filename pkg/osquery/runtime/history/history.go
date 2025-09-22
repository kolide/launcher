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
	instances []*instance
	store     types.GetterSetter
}

type NoInstancesError struct{}

func (c NoInstancesError) Error() string {
	return "no osquery instances have been added to history"
}

// InitHistory loads the osquery instance history from bbolt DB if exists, sets up bucket if it does not
func InitHistory(store types.GetterSetter) (*History, error) {
	history := History{
		instances: make([]*instance, 0),
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

// latestInstance is our internal helper for grabbing the latest history instance
// by registration id. it does not lock history because many of our exposed methods
// need to do further manipulation after grabbing the instance here, so that
// responsibility is maintained outside of this
func (h *History) latestInstance(registrationId string) (*instance, error) {
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

// LatestInstanceStats provides a map[string]string copy of our latest instance
// for the provided registration id
func (h *History) LatestInstanceStats(registrationId string) (map[string]string, error) {
	h.Lock()
	defer h.Unlock()

	instance, err := h.latestInstance(registrationId)
	if err != nil {
		return nil, err
	}

	return instance.toMap(), nil
}

// LatestInstanceId provides the instance id (queried from osquery) for our
// latest history instance by registration id
func (h *History) LatestInstanceId(registrationId string) (string, error) {
	h.Lock()
	defer h.Unlock()

	instance, err := h.latestInstance(registrationId)
	if err != nil {
		return "", err
	}

	return instance.InstanceId, nil
}

// LatestInstanceUptimeMinutes calculates the number of minutes since StartTime for our
// latest history instance by registration id
func (h *History) LatestInstanceUptimeMinutes(registrationId string) (int64, error) {
	h.Lock()
	defer h.Unlock()

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

// NewInstance adds a new instance to the osquery instance history after setting
// all available metadata and saves this new instance internally
func (h *History) NewInstance(registrationId string, runId string) error {
	h.Lock()
	defer h.Unlock()

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	newInstance := &instance{
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

// SetConnected finds the target instance by the provided run id, and utilizes the
// provided querier to set the proper instance id and version before saving the updates
// internally
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

// SetExited finds the target instance by the provided run id, notes the exit
// time and exitError (if provided), and saves the updates to our internal history
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

func (h *History) addInstanceToHistory(newInstance *instance) {
	if h.instances == nil {
		h.instances = []*instance{newInstance}
		return
	}

	h.instances = append(h.instances, newInstance)

	if len(h.instances) >= maxInstances {
		h.instances = h.instances[len(h.instances)-maxInstances:]
	}
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
