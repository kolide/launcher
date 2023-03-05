package checkpoint

import (
	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
)

var serverProvidedDataKeys = []string{
	"munemo",
	"organization_id",
	"device_id",
	"remote_ip",
}

// logServerProvidedData sends a subset of the server data into the checkpoint logs. This iterates over the
// desired keys, as a way to handle missing values.
func (c *checkPointer) logServerProvidedData() {
	data := make(map[string]string, len(serverProvidedDataKeys))

	if err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
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
