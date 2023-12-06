package checkups

import (
	"context"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

var serverProvidedDataKeys = []string{
	"munemo",
	"organization_id",
	"device_id",
	"remote_ip",
	"tombstone_id",
}

type serverDataCheckup struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]any
}

func (sdc *serverDataCheckup) Data() any             { return sdc.data }
func (sdc *serverDataCheckup) ExtraFileName() string { return "" }
func (sdc *serverDataCheckup) Name() string          { return "Server Data" }
func (sdc *serverDataCheckup) Status() Status        { return sdc.status }
func (sdc *serverDataCheckup) Summary() string       { return sdc.summary }

func (sdc *serverDataCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	store := sdc.k.ServerProvidedDataStore()
	sdc.data = make(map[string]any)

	if store == nil {
		sdc.status = Warning
		sdc.summary = "no server_data store in knapsack"
		return nil
	}

	// set up the default failure states, we will overwrite when we get the required data
	sdc.status = Failing
	sdc.summary = "unable to collect server data"
	for _, key := range serverProvidedDataKeys {
		val, err := store.Get([]byte(key))
		if err != nil {
			sdc.data[key] = err.Error()
			continue
		}

		if key == "device_id" && string(val) != "" {
			sdc.status = Passing
			sdc.summary = "successfully collected server data"
		}

		sdc.data[key] = string(val)
	}

	return nil
}
