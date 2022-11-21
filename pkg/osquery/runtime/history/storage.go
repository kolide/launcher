package history

import (
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"
)

const (
	osqueryHistoryInstanceKey = "osquery_instance_history"
)

type NoDbError struct{}

func (e NoDbError) Error() string {
	return "database is nil for osquery instance history, not persisting"
}

func (h *History) load() error {
	if h.db == nil {
		return NoDbError{}
	}

	var instancesBytes []byte

	if err := h.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osqueryHistoryInstanceKey))
		instancesBytes = b.Get([]byte(osqueryHistoryInstanceKey))
		return nil
	}); err != nil {
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

	if h.db == nil {
		return NoDbError{}
	}

	instancesBytes, err := json.Marshal(h.instances)
	if err != nil {
		return fmt.Errorf("error marshalling osquery_instance_history: %w", err)
	}

	if err := h.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osqueryHistoryInstanceKey))
		if err := b.Put([]byte(osqueryHistoryInstanceKey), instancesBytes); err != nil {
			return fmt.Errorf("error writing osquery_instance_history to db: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error writing osquery_instance_history to db: %w", err)
	}

	return nil
}

func createBboltBucketIfNotExists(db *bbolt.DB) error {
	if db == nil {
		return NoDbError{}
	}

	// Create Bolt buckets as necessary
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(osqueryHistoryInstanceKey)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error creating osquery_instance_history bucket: %w", err)
	}

	return nil
}
