package agent

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
)

type dbResetRecord struct {
	NodeKey        string   `json:"node_key"`
	PubKeys        [][]byte `json:"pub_keys"`
	Serial         string   `json:"serial"`
	HardwareUUID   string   `json:"hardware_uuid"`
	Munemo         string   `json:"munemo"`
	DeviceID       string   `json:"device_id"`
	RemoteIP       string   `json:"remote_ip"`
	TombstoneID    string   `json:"tombstone_id"`
	ResetTimestamp int64    `json:"reset_timestamp"`
	ResetReason    string   `json:"reset_reason"`
}

var (
	hostDataKeySerial       = []byte("serial")
	hostDataKeyHardwareUuid = []byte("hardware_uuid")
	hostDataKeyMunemo       = []byte("munemo")
	hostDataKeyResetRecords = []byte("reset_records")
	hostDataKeyMachineGuid  = []byte("machine_guid") // used for windows only, because the hardware_uuid is not stable

	ErrNewHardwareDetected = errors.New("need to reload launcher: hardware change detected and database wiped")
)

const (
	resetReasonNewHardwareOrEnrollmentDetected = "launcher detected new hardware or enrollment"
)

type UninitializedStorageError struct{}

func (use UninitializedStorageError) Error() string {
	return "storage is uninitialized in knapsack"
}

type hardwareChangeDetector struct {
	slogger     *slog.Logger
	k           types.Knapsack
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func NewHardwareChangeDetector(k types.Knapsack, slogger *slog.Logger) *hardwareChangeDetector {
	return &hardwareChangeDetector{
		slogger:     slogger.With("component", "hardware_change_detector"),
		k:           k,
		interrupt:   make(chan struct{}),
		interrupted: &atomic.Bool{},
	}
}

func (h *hardwareChangeDetector) Execute() error {
	if remediationOccurred := detectAndRemediateHardwareChange(context.TODO(), h.k, h.slogger); remediationOccurred {
		h.slogger.Log(context.TODO(), slog.LevelInfo,
			"hardware change detected and database wiped, sending shutdown request to launcher",
		)
		return ErrNewHardwareDetected
	}

	// We're done with our check -- nothing to do now except wait to shut down whenever launcher shuts down next.
	<-h.interrupt
	return nil
}

func (h *hardwareChangeDetector) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if h.interrupted.Swap(true) {
		return
	}

	h.interrupt <- struct{}{}
}

// detectAndRemediateHardwareChange checks to see if the hardware this installation is running on
// has changed, by checking current hardware-identifying information against stored data in the
// HostDataStore. If the hardware-identifying information has changed, it logs the change; if the
// ResetOnHardwareChangeEnabled feature flag is enabled, then it will reset the database. Returns
// a bool of whether remediation occurred.
func detectAndRemediateHardwareChange(ctx context.Context, k types.Knapsack, slogger *slog.Logger) bool {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	serialChanged := false
	hardwareUUIDChanged := false
	munemoChanged := false
	machineGuidChanged := false

	defer func() {
		slogger.Log(ctx, slog.LevelDebug, "finished check to see if database should be reset",
			"serial", serialChanged,
			"hardware_uuid", hardwareUUIDChanged,
			"machine_guid", machineGuidChanged,
			"munemo", munemoChanged,
		)
	}()

	currentSerial, currentHardwareUUID, err := currentSerialAndHardwareUUID(ctx, k)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get current serial and hardware UUID", "err", err)
	} else {
		serialChanged = valueChanged(ctx, k, slogger, currentSerial, hostDataKeySerial)
		hardwareUUIDChanged = valueChanged(ctx, k, slogger, currentHardwareUUID, hostDataKeyHardwareUuid)
	}

	currentMachineGuid, err := currentMachineGuid(ctx, k)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get current machine GUID", "err", err)
	} else {
		machineGuidChanged = valueChanged(ctx, k, slogger, currentMachineGuid, hostDataKeyMachineGuid)
	}

	// Collect munemo for logging/record-keeping purposes only -- we don't use it to determine whether
	// we should perform a reset
	currentTenantMunemo, err := currentMunemo(k)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get current munemo", "err", err)
	} else {
		munemoChanged = valueChanged(ctx, k, slogger, currentTenantMunemo, hostDataKeyMunemo)
	}

	remediationRequired := serialChanged && hardwareUUIDChanged
	if runtime.GOOS == "windows" {
		// note that machineGuid is only collected for windows; machineGuidChanged will only ever have a meaningful value for Windows.
		// serialChanged and hardwareUUIDChanged are NOT meaningful on Windows -- we see a lot of flapping there that does not indicate
		// actual hardware changes.
		remediationRequired = machineGuidChanged
	}
	remediationOccurred := false
	if remediationRequired {
		slogger.Log(ctx, slog.LevelInfo,
			"detected hardware change",
			"serial_changed", serialChanged,
			"hardware_uuid_changed", hardwareUUIDChanged,
			"tenant_munemo_changed", munemoChanged,
			"machine_guid_changed", machineGuidChanged,
			"reset_on_hardware_change_enabled", k.ResetOnHardwareChangeEnabled(),
		)

		if k.ResetOnHardwareChangeEnabled() {
			if err := ResetDatabase(ctx, k, slogger, resetReasonNewHardwareOrEnrollmentDetected); err != nil {
				slogger.Log(ctx, slog.LevelWarn,
					"failed to reset database",
					"err", err,
				)
			} else {
				slogger.Log(ctx, slog.LevelInfo,
					"successfully reset the database after hardware change detected",
				)
				remediationOccurred = true
			}
		}
	}

	// Update store for record-keeping purposes and future checks
	if serialChanged || remediationOccurred {
		if err := k.PersistentHostDataStore().Set(hostDataKeySerial, []byte(currentSerial)); err != nil {
			slogger.Log(ctx, slog.LevelWarn, "could not set serial in host data store", "err", err)
		}
	}
	if hardwareUUIDChanged || remediationOccurred {
		if err := k.PersistentHostDataStore().Set(hostDataKeyHardwareUuid, []byte(currentHardwareUUID)); err != nil {
			slogger.Log(ctx, slog.LevelWarn, "could not set hardware UUID in host data store", "err", err)
		}
	}
	if munemoChanged || remediationOccurred {
		if err := k.PersistentHostDataStore().Set(hostDataKeyMunemo, []byte(currentTenantMunemo)); err != nil {
			slogger.Log(ctx, slog.LevelWarn, "could not set munemo in host data store", "err", err)
		}
	}
	if machineGuidChanged || remediationOccurred {
		if err := k.PersistentHostDataStore().Set(hostDataKeyMachineGuid, []byte(currentMachineGuid)); err != nil {
			slogger.Log(ctx, slog.LevelWarn, "could not set machine GUID in host data store", "err", err)
		}
	}

	return remediationOccurred
}

func GetResetRecords(ctx context.Context, k types.Knapsack) ([]dbResetRecord, error) {
	resetRecords := make([]dbResetRecord, 0)
	if k.PersistentHostDataStore() == nil {
		return resetRecords, UninitializedStorageError{}
	}

	resetRecordsRaw, err := k.PersistentHostDataStore().Get(hostDataKeyResetRecords)
	if err != nil {
		return resetRecords, err
	}

	if len(resetRecordsRaw) == 0 {
		return resetRecords, nil
	}

	if err := json.Unmarshal(resetRecordsRaw, &resetRecords); err != nil {
		return resetRecords, err
	}

	return resetRecords, nil
}

func ResetDatabase(ctx context.Context, k types.Knapsack, slogger *slog.Logger, resetReason string) error {
	backup, err := prepareDatabaseResetRecords(ctx, k, slogger, resetReason)
	if err != nil {
		slogger.Log(ctx, slog.LevelError, "could not prepare db reset records", "err", err)
		return err
	}

	if err := wipeDatabase(k); err != nil {
		slogger.Log(ctx, slog.LevelError, "could not wipe database", "err", err)
		return err
	}

	// Store the backup data
	if err := k.PersistentHostDataStore().Set(hostDataKeyResetRecords, backup); err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not store db reset records", "err", err)
		return err
	}

	return nil
}

// currentSerialAndHardwareUUID queries osquery for the required information.
func currentSerialAndHardwareUUID(ctx context.Context, k types.Knapsack) (string, string, error) {
	osqPath := k.LatestOsquerydPath(ctx)
	if osqPath == "" {
		return "", "", errors.New("could not get osqueryd path from knapsack")
	}

	query := `
	SELECT
		system_info.hardware_serial,
		system_info.uuid AS hardware_uuid
	FROM
		system_info;
`

	var respBytes bytes.Buffer
	var stderrBytes bytes.Buffer

	osqProc, err := runsimple.NewOsqueryProcess(osqPath, runsimple.WithStdout(&respBytes), runsimple.WithStderr(&stderrBytes))
	if err != nil {
		return "", "", fmt.Errorf("could not create osquery process to determine hardware UUID or serial: %w", err)
	}

	osqCtx, osqCancel := context.WithTimeout(ctx, 10*time.Second)
	defer osqCancel()

	if sqlErr := osqProc.RunSql(osqCtx, []byte(query)); osqCtx.Err() != nil {
		return "", "", fmt.Errorf(
			"querying hardware UUID and serial returned ctx error: `%w` with stdout `%s` and stderr `%s`",
			osqCtx.Err(), respBytes.String(), stderrBytes.String(),
		)
	} else if sqlErr != nil {
		return "", "", fmt.Errorf(
			"querying hardware UUID and serial returned error: `%w` with stdout `%s` and stderr `%s`",
			sqlErr, respBytes.String(), stderrBytes.String(),
		)
	}

	var resp []map[string]string
	if err := json.Unmarshal(respBytes.Bytes(), &resp); err != nil {
		return "", "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(resp) < 1 {
		return "", "", errors.New("no rows returned")
	}

	serial, ok := resp[0]["hardware_serial"]
	if !ok {
		return "", "", errors.New("hardware_serial missing from results")
	}

	hardwareUUID, ok := resp[0]["hardware_uuid"]
	if !ok {
		return "", "", errors.New("hardware_uuid missing from results")
	}

	return serial, hardwareUUID, nil
}

// valueChanged checks the knapsack for the given data and compares it to
// currentValue. A value is considered changed only if the stored value was
// previously set.
func valueChanged(ctx context.Context, k types.Knapsack, slogger *slog.Logger, currentValue string, dataKey []byte) bool {
	storedValue, err := k.PersistentHostDataStore().Get(dataKey)
	if err != nil {
		slogger.Log(ctx, slog.LevelError, "could not get stored value", "err", err, "key", string(dataKey))
		return false // assume no change
	}

	if len(storedValue) == 0 && len(currentValue) > 0 {
		slogger.Log(ctx, slog.LevelDebug, "value not previously stored, storing now", "key", string(dataKey))
		if err := k.PersistentHostDataStore().Set(dataKey, []byte(currentValue)); err != nil {
			slogger.Log(ctx, slog.LevelError, "could not store value", "err", err, "key", string(dataKey))
		}
		return false
	}

	if storedValue != nil && currentValue != string(storedValue) {
		slogger.Log(ctx, slog.LevelInfo, "hardware- or enrollment-identifying value has changed",
			"key", string(dataKey),
			"old_value", string(storedValue),
			"new_value", currentValue,
		)
		return true
	}

	return false
}

// currentMunemo retrieves the enrollment secret from either the knapsack or the filesystem,
// depending on launcher configuration, and then parses the tenant munemo from it.
func currentMunemo(k types.Knapsack) (string, error) {
	registrations, err := k.Registrations()
	if err != nil {
		return "", fmt.Errorf("getting registrations from knapsack: %w", err)
	}
	if len(registrations) == 0 {
		return "", errors.New("no registrations in knapsack")
	}

	// For now, we just want the default registration.
	for _, r := range registrations {
		if r.RegistrationID == types.DefaultRegistrationID {
			return r.Munemo, nil
		}
	}

	return "", fmt.Errorf("no registration found for `%s` registration ID, cannot find munemo", types.DefaultRegistrationID)
}

// prepareDatabaseResetRecords retrieves the data we want to preserve from various db stores
// as a record of the current state of this database before reset. It appends this record
// to previous records if they exist, and returns the collection ready for storage.
func prepareDatabaseResetRecords(ctx context.Context, k types.Knapsack, slogger *slog.Logger, resetReason string) ([]byte, error) { // nolint:unused
	nodeKeys := make([]string, 0)
	for _, registrationId := range k.RegistrationIDs() {
		nodeKey, err := k.ConfigStore().Get(storage.KeyByIdentifier([]byte("nodeKey"), storage.IdentifierTypeRegistration, []byte(registrationId)))
		if err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"could not get node key from store",
				"registration_id", registrationId,
				"err", err,
			)
			continue
		}
		nodeKeys = append(nodeKeys, string(nodeKey))
	}
	nodeKey := strings.Join(nodeKeys, ",")

	localPubKey, err := getLocalPubKey(k)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get local pubkey from store", "err", err)
	}

	serial, err := k.PersistentHostDataStore().Get(hostDataKeySerial)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get serial from store", "err", err)
	}

	hardwareUuid, err := k.PersistentHostDataStore().Get(hostDataKeyHardwareUuid)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get hardware uuid from store", "err", err)
	}

	munemo, err := k.PersistentHostDataStore().Get(hostDataKeyMunemo)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get munemo from store", "err", err)
	}

	deviceId, err := k.ServerProvidedDataStore().Get([]byte("device_id"))
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get device id from store", "err", err)
	}

	remoteIp, err := k.ServerProvidedDataStore().Get([]byte("remote_ip"))
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get remote ip from store", "err", err)
	}

	tombstoneId, err := k.ServerProvidedDataStore().Get([]byte("tombstone_id"))
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn, "could not get tombstone id from store", "err", err)
	}

	dataToStore := dbResetRecord{
		NodeKey:        nodeKey,
		PubKeys:        [][]byte{localPubKey},
		Serial:         string(serial),
		HardwareUUID:   string(hardwareUuid),
		Munemo:         string(munemo),
		DeviceID:       string(deviceId),
		RemoteIP:       string(remoteIp),
		TombstoneID:    string(tombstoneId),
		ResetTimestamp: time.Now().Unix(),
		ResetReason:    resetReason,
	}

	previousHostData, err := GetResetRecords(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("getting previous host data from store: %w", err)
	}

	previousHostData = append(previousHostData, dataToStore)
	hostDataCollectionRaw, err := json.Marshal(previousHostData)
	if err != nil {
		return nil, fmt.Errorf("marshalling host data for storage: %w", err)
	}

	return hostDataCollectionRaw, nil
}

// getLocalPubKey retrieves the local database key, parses it, and returns
// the pubkey.
func getLocalPubKey(k types.Knapsack) ([]byte, error) { // nolint:unused
	localEccKeyRaw, err := k.ConfigStore().Get([]byte("localEccKey"))
	if err != nil {
		return nil, fmt.Errorf("getting raw key from config store: %w", err)
	}

	localEccKey, err := x509.ParseECPrivateKey(localEccKeyRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing key: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(localEccKey.Public())
	if err != nil {
		return nil, fmt.Errorf("marshalling pubkey: %w", err)
	}

	return pubKeyBytes, nil
}

// wipeDatabase iterates over all stores in the database, deleting all keys from
// each one.
func wipeDatabase(k types.Knapsack) error {
	for storeName, store := range k.Stores() {
		if err := store.DeleteAll(); err != nil {
			return fmt.Errorf("deleting keys in store %s: %w", storeName, err)
		}
	}
	return nil
}
