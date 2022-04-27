package history

import (
	"encoding/json"

	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	osqueryHistoryInstanceKey = "osquery_instance_history"
)

func (h *History) load() error {
	var instancesBytes []byte

	err := h.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osqueryHistoryInstanceKey))
		instancesBytes = b.Get([]byte(osqueryHistoryInstanceKey))
		return nil
	})

	if err != nil {
		return errors.Wrap(err, "error reading osquery_instance_history from db")
	}

	var instances []*Instance

	if instancesBytes != nil {
		err = json.Unmarshal(instancesBytes, &instances)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling osquery_instance_history")
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
		return errors.Wrap(err, "error marshalling osquery_instance_history")
	}

	err = h.db.Update(func(tx *bbolt.Tx) error {

		b := tx.Bucket([]byte(osqueryHistoryInstanceKey))
		err = b.Put([]byte(osqueryHistoryInstanceKey), instancesBytes)
		if err != nil {
			return errors.Wrap(err, "error writing osquery_instance_history to db")
		}
		return nil
	})

	if err != nil {
		return errors.Wrap(err, "error writing osquery_instance_history to db")
	}

	return nil
}

func createBboltBucket(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(osqueryHistoryInstanceKey))
		if err != nil {
			return errors.Wrap(err, "error creating osquery_instance_history bucket")
		}
		return nil
	})
}
