package kolide_server_data

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
)

// UpdateHandler processes control server updates for the server-provided data bucket
type UpdateHandler struct {
	db *bbolt.DB
}

func NewUpdateHandler(db *bbolt.DB) *UpdateHandler {
	c := &UpdateHandler{
		db: db,
	}

	return c
}

func (c *UpdateHandler) Update(data io.Reader) {
	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		// return fmt.Errorf("failed to decode server data json: %w", err)
	}

	c.db.Update(func(tx *bbolt.Tx) error {
		// Clear the bucket first
		tx.DeleteBucket([]byte(osquery.ServerProvidedDataBucket))

		bucket, err := tx.CreateBucketIfNotExists([]byte(osquery.ServerProvidedDataBucket))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		for key, value := range kvPairs {
			if err := bucket.Put([]byte(key), []byte(value)); err != nil {
				return fmt.Errorf("storing key: %w", err)
			}
		}

		return nil
	})

}
