package checkpoint

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
	data := make(map[string]string, len(serverProvidedDataKeys))

	for _, key := range serverProvidedDataKeys {
		val, err := c.store.Get([]byte(key))
		if err != nil {
			c.logger.Log("server_data", "error fetching data", "err", err)
		}
		if val == nil {
			continue
		}

		data[key] = string(val)
	}
}
