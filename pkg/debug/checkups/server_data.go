package checkups

import (
	"context"
	"fmt"
	"io"

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
	data    map[string]string
}

func (sdc *serverDataCheckup) Data() any             { return sdc.data }
func (sdc *serverDataCheckup) ExtraFileName() string { return "" }
func (sdc *serverDataCheckup) Name() string          { return "Server Data" }
func (sdc *serverDataCheckup) Status() Status        { return sdc.status }
func (sdc *serverDataCheckup) Summary() string       { return sdc.summary }

func (sdc *serverDataCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	db := sdc.k.BboltDB()
	sdc.data = make(map[string]string, len(serverProvidedDataKeys))

	if db == nil {
		return nil
	}

	accessedBucket, missingValues := false, false

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storage.ServerProvidedDataStore))
		if b == nil {
			return nil
		}

		accessedBucket = true
		for _, key := range serverProvidedDataKeys {
			val := b.Get([]byte(key))
			if val == nil {
				missingValues = true
				continue
			}

			sdc.data[key] = string(val)
		}

		return nil
	}); err != nil {
		return err
	}

	sdc.status = Informational
	if !accessedBucket {
		sdc.summary = fmt.Sprintf("unable to view bucket: %s", storage.ServerProvidedDataStore)
	} else if missingValues {
		sdc.summary = fmt.Sprintf("successfully connected to %s bucket, but some values are missing", storage.ServerProvidedDataStore)
	} else {
		sdc.summary = fmt.Sprintf("successfully gathered all data values from %s bucket", storage.ServerProvidedDataStore)
	}

	return nil
}
