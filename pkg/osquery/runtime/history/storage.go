package history

import (
	"encoding/json"

	"github.com/pkg/errors"
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
		return errors.Wrap(err, "error reading osquery_instance_history from db")
	}

	var instances []*Instance

	if instancesBytes != nil {
		if err := json.Unmarshal(instancesBytes, &instances); err != nil {
			return errors.Wrap(err, "error unmarshalling osquery_instance_history")
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
		return errors.Wrap(err, "error marshalling osquery_instance_history")
	}

	if err := h.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osqueryHistoryInstanceKey))
		if err := b.Put([]byte(osqueryHistoryInstanceKey), instancesBytes); err != nil {
			return errors.Wrap(err, "error writing osquery_instance_history to db")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "error writing osquery_instance_history to db")
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
		return errors.Wrap(err, "error creating osquery_instance_history bucket")
	}

	return nil
}
