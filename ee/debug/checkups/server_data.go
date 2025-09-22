package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
func (sdc *serverDataCheckup) ExtraFileName() string { return "metadata.json" }
func (sdc *serverDataCheckup) Name() string          { return "Server Data" }
func (sdc *serverDataCheckup) Status() Status        { return sdc.status }
func (sdc *serverDataCheckup) Summary() string       { return sdc.summary }

func (sdc *serverDataCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	store := sdc.k.ServerProvidedDataStore()
	sdc.data = make(map[string]any)

	if err := sdc.addMetadata(extraFH); err != nil {
		sdc.status = Warning
		sdc.summary += "; failed to add metadata.json"
		return fmt.Errorf("adding metadata.json: %w", err)
	}

	if store == nil {
		// We are probably running standalone instead of in situ
		sdc.status = Informational
		sdc.summary = "server_data not available"
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

func (sdc *serverDataCheckup) addMetadata(extraFH io.Writer) error {
	rootDir := sdc.k.RootDirectory()
	metadataPath := filepath.Join(rootDir, "metadata.json")

	metadataFile, err := os.Open(metadataPath)
	if err != nil {
		return fmt.Errorf("opening metadata.json: %w", err)
	}
	defer metadataFile.Close()

	// Copy the contents of metadata.json directly to extraFH
	if _, err := io.Copy(extraFH, metadataFile); err != nil {
		return fmt.Errorf("writing metadata.json to extra file handler: %w", err)
	}

	return nil
}
