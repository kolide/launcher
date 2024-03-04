package checkups

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
)

type (
	uninstallHistoryCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (hc *uninstallHistoryCheckup) Data() any             { return hc.data }
func (hc *uninstallHistoryCheckup) ExtraFileName() string { return "" }
func (hc *uninstallHistoryCheckup) Name() string          { return "Uninstall History" }
func (hc *uninstallHistoryCheckup) Status() Status        { return hc.status }
func (hc *uninstallHistoryCheckup) Summary() string       { return hc.summary }

func (hc *uninstallHistoryCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hc.data = make(map[string]any)
	if hc.k.PersistentHostDataStore() == nil {
		hc.status = Informational
		hc.summary = "Unable to access uninstall history"
		return nil
	}

	resetRecordsRaw, err := hc.k.PersistentHostDataStore().Get(agent.HostDataKeyResetRecords)
	if err != nil {
		hc.status = Erroring
		hc.summary = "Unable to gather previous host data from store"
		hc.data["error"] = err.Error()
		return nil
	}

	var resetRecords []agent.DBResetRecord
	if len(resetRecordsRaw) == 0 {
		hc.status = Informational
		hc.summary = "No installation history exists for this device"
		return nil
	}

	if err := json.Unmarshal(resetRecordsRaw, &resetRecords); err != nil {
		hc.status = Erroring
		hc.summary = "Unable to unmarshal previous host data from store"
		hc.data["error"] = err.Error()
		return nil
	}

	for _, uninstallRecord := range resetRecords {
		resetTimeKey := time.Unix(uninstallRecord.ResetTimestamp, 0)
		hc.data[resetTimeKey.Format(time.RFC3339)] = map[string]any{
			"serial":          uninstallRecord.Serial,
			"hardware_uuid":   uninstallRecord.HardwareUUID,
			"munemo":          uninstallRecord.Munemo,
			"device_id":       uninstallRecord.DeviceID,
			"remote_ip":       uninstallRecord.RemoteIP,
			"tombstone_id":    uninstallRecord.TombstoneID,
			"reset_timestamp": resetTimeKey,
			"reset_reason":    uninstallRecord.ResetReason,
		}
	}

	hc.status = Passing
	hc.summary = "Successfully collected uninstallation history"

	return nil
}
