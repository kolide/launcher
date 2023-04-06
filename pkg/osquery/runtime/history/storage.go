package history

import (
	"encoding/json"
	"fmt"
)

const (
	osqueryHistoryInstanceKey = "osquery_instance_history"
)

func (h *History) load() error {
	var instancesBytes []byte

	instancesBytes, err := h.store.Get([]byte(osqueryHistoryInstanceKey))
	if err != nil {
		return fmt.Errorf("error reading osquery_instance_history from db: %w", err)
	}

	var instances []*Instance

	if instancesBytes != nil {
		if err := json.Unmarshal(instancesBytes, &instances); err != nil {
			return fmt.Errorf("error unmarshalling osquery_instance_history: %w", err)
		}
	} else {
		return nil
	}

	h.instances = instances
	return nil
}

func (h *History) save() error {
	instancesBytes, err := json.Marshal(h.instances)
	if err != nil {
		return fmt.Errorf("error marshalling osquery_instance_history: %w", err)
	}

	if err := h.store.Set([]byte(osqueryHistoryInstanceKey), instancesBytes); err != nil {
		return fmt.Errorf("error writing osquery_instance_history to storage: %w", err)
	}

	return nil
}
