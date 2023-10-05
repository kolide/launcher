package checkups

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
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

func (sdc *serverDataCheckup) Data() map[string]any  { return sdc.data }
func (sdc *serverDataCheckup) ExtraFileName() string { return "" }
func (sdc *serverDataCheckup) Name() string          { return "Server Data" }
func (sdc *serverDataCheckup) Status() Status        { return sdc.status }
func (sdc *serverDataCheckup) Summary() string       { return sdc.summary }

func (sdc *serverDataCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	db := sdc.k.BboltDB()
	sdc.data = make(map[string]any)

	if db == nil {
		sdc.status = Warning
		sdc.summary = "no bbolt DB connection in knapsack"
		return nil
	}

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storage.ServerProvidedDataStore))
		if b == nil {
			return fmt.Errorf("unable to access bbolt bucket (%s)", storage.ServerProvidedDataStore)
		}

		for _, key := range serverProvidedDataKeys {
			val := b.Get([]byte(key))
			if val == nil {
				continue
			}

			sdc.data[key] = string(val)
		}

		return nil
	}); err != nil {
		sdc.status = Erroring
		sdc.data["error"] = err.Error()
		sdc.summarize()
		return nil
	}

	sdc.status = Passing
	sdc.summarize()

	return nil
}

func (sdc *serverDataCheckup) summarize() {
	summary := make([]string, 0)

	for k, v := range sdc.data {
		summary = append(summary, fmt.Sprintf("%s: %s", k, v))
	}

	sdc.summary = fmt.Sprintf("collected server data: [%s]", strings.Join(summary, ", "))
}
