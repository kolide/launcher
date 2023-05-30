package checkpoint

import (
	"github.com/kolide/launcher/pkg/agent/storage"
	"go.etcd.io/bbolt"
)

var serverProvidedDataKeys = []string{
	"munemo",
	"organization_id",
	"device_id",
	"remote_ip",
	"tombstone_id",
}

// logServerProvidedData sends a subset of the server data into the checkpoint logs. This iterates over the
// desired keys, as a way to handle missing values.
func (c *checkPointer) logServerProvidedData() {
	db := c.knapsack.BboltDB()
	if db == nil {
		return
	}

	data := make(map[string]string, len(serverProvidedDataKeys))

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storage.ServerProvidedDataStore))
		if b == nil {
			return nil
		}

		for _, key := range serverProvidedDataKeys {
			val := b.Get([]byte(key))
			if val == nil {
				continue
			}

			data[key] = string(val)
		}

		return nil
	}); err != nil {
		c.logger.Log("server_data", "error fetching data", "err", err)
	}

	c.logger.Log("server_data", data)
}
