package agent

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	"github.com/kolide/launcher/pkg/traces"
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
)

const (
	resetReasonNewHardwareOrEnrollmentDetected = "launcher detected new hardware or enrollment"
)

type UninitializedStorageError struct{}

func (use UninitializedStorageError) Error() string {
	return "storage is uninitialized in knapsack"
}

// DetectAndRemediateHardwareChange checks to see if the hardware this installation is running on
// has changed, by checking current hardware- and enrollment- identifying information against
// stored data in the HostDataStore. If the hardware- or enrollment-identifying information
// has changed, it logs the change. In the future, it will take a backup of the database, and
// then clear all data from it.
func DetectAndRemediateHardwareChange(ctx context.Context, k types.Knapsack) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	serialChanged := false
	hardwareUUIDChanged := false
	munemoChanged := false

	defer func() {
		k.Slogger().Log(ctx, slog.LevelDebug, "finished check to see if database should be reset...",
			"serial", serialChanged,
			"hardware_uuid", hardwareUUIDChanged,
			"munemo", munemoChanged,
		)
	}()

	currentSerial, currentHardwareUUID, err := currentSerialAndHardwareUUID(ctx, k)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get current serial and hardware UUID", "err", err)
	} else {
		serialChanged = valueChanged(ctx, k, currentSerial, hostDataKeySerial)
		hardwareUUIDChanged = valueChanged(ctx, k, currentHardwareUUID, hostDataKeyHardwareUuid)
	}

	currentTenantMunemo, err := currentMunemo(k)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get current munemo", "err", err)
	} else {
		munemoChanged = valueChanged(ctx, k, currentTenantMunemo, hostDataKeyMunemo)
	}

	if serialChanged || hardwareUUIDChanged || munemoChanged {
		// In the future, we can proceed with backing up and resetting the database.
		// For now, we are only logging that we detected the change until we have a dependable
		// hardware change detection method - see issue here https://github.com/kolide/launcher/issues/1346
		/*
			k.Slogger().Log(ctx, slog.LevelWarn, "resetting the database",
				"serial_changed", serialChanged,
				"hardware_uuid_changed", hardwareUUIDChanged,
				"tenant_munemo_changed", munemoChanged,
			)

			if err := ResetDatabase(ctx, k, resetReasonNewHardwareOrEnrollmentDetected); err != nil {
				k.Slogger().Log(ctx, slog.LevelError, "failed to reset database", "err", err)
			}
		*/

		// Cache hardware and rollout data for future checks
		if err := k.PersistentHostDataStore().Set(hostDataKeySerial, []byte(currentSerial)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set serial in host data store", "err", err)
		}
		if err := k.PersistentHostDataStore().Set(hostDataKeyHardwareUuid, []byte(currentHardwareUUID)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set hardware UUID in host data store", "err", err)
		}
		if err := k.PersistentHostDataStore().Set(hostDataKeyMunemo, []byte(currentTenantMunemo)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set munemo in host data store", "err", err)
		}
	}
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

func ResetDatabase(ctx context.Context, k types.Knapsack, resetReason string) error {
	backup, err := prepareDatabaseResetRecords(ctx, k, resetReason)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelError, "could not prepare db reset records", "err", err)
		return err
	}

	if err := wipeDatabase(ctx, k); err != nil {
		k.Slogger().Log(ctx, slog.LevelError, "could not wipe database", "err", err)
		return err
	}

	// Store the backup data
	if err := k.PersistentHostDataStore().Set(hostDataKeyResetRecords, backup); err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not store db reset records", "err", err)
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
func valueChanged(ctx context.Context, k types.Knapsack, currentValue string, dataKey []byte) bool {
	storedValue, err := k.PersistentHostDataStore().Get(dataKey)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelError, "could not get stored value", "err", err, "key", string(dataKey))
		return false // assume no change
	}

	if len(storedValue) == 0 {
		k.Slogger().Log(ctx, slog.LevelDebug, "value not previously stored, storing now", "key", string(dataKey))
		if err := k.PersistentHostDataStore().Set(dataKey, []byte(currentValue)); err != nil {
			k.Slogger().Log(ctx, slog.LevelError, "could not store value", "err", err, "key", string(dataKey))
		}
		return false
	}

	if storedValue != nil && currentValue != string(storedValue) {
		k.Slogger().Log(ctx, slog.LevelInfo, "hardware- or enrollment-identifying value has changed",
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
	var enrollSecret string
	if k.EnrollSecret() != "" {
		enrollSecret = k.EnrollSecret()
	} else if k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(k.EnrollSecretPath())
		if err != nil {
			return "", fmt.Errorf("could not read secret at enroll_secret_path %s: %w", k.EnrollSecretPath(), err)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	} else {
		return "", errors.New("enroll secret and secret path both unset")
	}

	// We cannot verify since we don't have the key, so we use ParseUnverified
	token, _, err := new(jwt.Parser).ParseUnverified(enrollSecret, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("parsing enroll secret jwt: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("enroll secret has no claims")
	}

	munemo, ok := claims["organization"]
	if !ok {
		return "", errors.New("enroll secret claims missing organization claim")
	}

	munemoStr, ok := munemo.(string)
	if !ok {
		return "", errors.New("munemo is unsupported type")
	}

	return munemoStr, nil
}

// prepareDatabaseResetRecords retrieves the data we want to preserve from various db stores
// as a record of the current state of this database before reset. It appends this record
// to previous records if they exist, and returns the collection ready for storage.
func prepareDatabaseResetRecords(ctx context.Context, k types.Knapsack, resetReason string) ([]byte, error) { // nolint:unused
	nodeKeys := make([]string, 0)
	for _, registrationId := range k.RegistrationIDs() {
		nodeKey, err := k.ConfigStore().Get(storage.KeyByIdentifier([]byte("nodeKey"), storage.IdentifierTypeRegistration, []byte(registrationId)))
		if err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn,
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
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get local pubkey from store", "err", err)
	}

	serial, err := k.PersistentHostDataStore().Get(hostDataKeySerial)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get serial from store", "err", err)
	}

	hardwareUuid, err := k.PersistentHostDataStore().Get(hostDataKeyHardwareUuid)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get hardware uuid from store", "err", err)
	}

	munemo, err := k.PersistentHostDataStore().Get(hostDataKeyMunemo)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get munemo from store", "err", err)
	}

	deviceId, err := k.ServerProvidedDataStore().Get([]byte("device_id"))
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get device id from store", "err", err)
	}

	remoteIp, err := k.ServerProvidedDataStore().Get([]byte("remote_ip"))
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get remote ip from store", "err", err)
	}

	tombstoneId, err := k.ServerProvidedDataStore().Get([]byte("tombstone_id"))
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get tombstone id from store", "err", err)
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
func wipeDatabase(ctx context.Context, k types.Knapsack) error {
	for storeName, store := range k.Stores() {
		if err := store.DeleteAll(); err != nil {
			return fmt.Errorf("deleting keys in store %s: %w", storeName, err)
		}
	}
	return nil
}
