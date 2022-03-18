package osquery_instance_history

import (
	"errors"
	"time"
)

const max_instances = 10

type History struct {
	instances []*Instance
}

// GetHistory returns the last 10 instances of osquery started / restarted by launcher
// each start / restart cycle is an entry
func GetHistory() []Instance {
	results := []Instance{}
	for _, v := range currentHistory.instances {
		results = append(results, *v)
	}
	return results
}

// CurrentInstance returns the current osquery instance
func CurrentInstance() Instance {
	return *currentHistory.currentInstance()
}

func (h *History) nextInstance() error {
	if h.currentInstance() != nil && h.currentInstance().ExitTime == "" {
		return errors.New("cannot create new instance while current instance does not have exit time")
	}

	newInstance := &Instance{
		StartTime: timeNow(),
	}

	h.setCurrentInstance(newInstance)

	return nil
}

func (h *History) currentInstance() *Instance {
	if h.instances != nil && len(h.instances) > 0 {
		return h.instances[len(h.instances)-1]
	}
	return nil
}

func (h *History) setCurrentInstance(instance *Instance) {
	if h.instances == nil {
		h.instances = []*Instance{instance}
		return
	}

	if len(h.instances) >= max_instances {
		h.instances = h.instances[1:]
	}

	h.instances = append(h.instances, instance)
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
