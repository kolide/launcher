package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
)

type oldHostData struct {
	NodeKey        string `json:"node_key"`
	LocalEccKey    string `json:"local_ecc_key"`
	Serial         string `json:"serial"`
	HardwareUUID   string `json:"hardware_uuid"`
	Munemo         string `json:"munemo"`
	DeviceID       string `json:"device_id"`
	RemoteIP       string `json:"remote_ip"`
	TombstoneID    string `json:"tombstone_id"`
	ResetTimestamp int64  `json:"reset_timestamp"`
}

var (
	hostDataKeySerial       = []byte("serial")
	hostDataKeyHardwareUuid = []byte("hardware_uuid")
	hostDataKeyMunemo       = []byte("munemo")

	hostDataKeyOldHostData = []byte("old_host_data")
)

// ResetDatabaseIfNeeded checks to see if the hardware this installation is running on
// has changed, by checking current hardware-identifying information against stored data
// in the HostDataStore. If the hardware-identifying information has changed, it performs
// a backup of the database, and then clears all data from it.
func ResetDatabaseIfNeeded(ctx context.Context, k types.Knapsack) {
	k.Slogger().Log(ctx, slog.LevelDebug, "checking to see if database should be reset...")

	serialChanged := false
	hardwareUUIDChanged := false
	currentSerial, currentHardwareUUID, err := currentSerialAndHardwareUUID(ctx, k)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get current serial and hardware UUID", "err", err)
	} else {
		serialChanged = valueChanged(k, currentSerial, hostDataKeySerial)
		hardwareUUIDChanged = valueChanged(k, currentHardwareUUID, hostDataKeyHardwareUuid)
	}

	munemoChanged := false
	currentTenantMunemo, err := currentMunemo(k)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get current munemo", "err", err)
	} else {
		munemoChanged = valueChanged(k, currentTenantMunemo, hostDataKeyMunemo)
	}

	if serialChanged || hardwareUUIDChanged || munemoChanged {
		k.Slogger().Log(ctx, slog.LevelWarn, "detected new hardware or rollout, backing up and resetting database",
			"serial_changed", serialChanged,
			"hardware_uuid_changed", hardwareUUIDChanged,
			"tenant_munemo_changed", munemoChanged)

		if err := takeDatabaseBackup(k); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not take database backup", "err", err)
		}

		wipeDatabase(k)

		// Cache data for future checks
		if err := k.HostDataStore().Set(hostDataKeySerial, []byte(currentSerial)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set serial in host data store", "err", err)
		}
		if err := k.HostDataStore().Set(hostDataKeyHardwareUuid, []byte(currentHardwareUUID)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set hardware UUID in host data store", "err", err)
		}
		if err := k.HostDataStore().Set(hostDataKeyMunemo, []byte(currentTenantMunemo)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set munemo in host data store", "err", err)
		}
	}
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
func valueChanged(k types.Knapsack, currentValue string, dataKey []byte) bool {
	storedValue, err := k.HostDataStore().Get(dataKey)
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelError, "could not get stored value", "err", err, "key", string(dataKey))
		return false // assume no change
	}

	if storedValue == nil {
		k.Slogger().Log(context.TODO(), slog.LevelDebug, "value not previously stored, storing now", "key", string(dataKey))
		if err := k.HostDataStore().Set(dataKey, []byte(currentValue)); err != nil {
			k.Slogger().Log(context.TODO(), slog.LevelError, "could not store value", "err", err, "key", string(dataKey))
		}
		return false
	}

	if storedValue != nil && currentValue != string(storedValue) {
		k.Slogger().Log(context.TODO(), slog.LevelInfo, "hardware-identifying value has changed", "key", string(dataKey))
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

// takeDatabaseBackup retrieves the data we want to preserve from various db stores
// as a snapshot of this db, appends it to previous snapshots if they exist, and
// stores the collection to the old_host_data key.
func takeDatabaseBackup(k types.Knapsack) error {
	nodeKey, err := k.ConfigStore().Get([]byte("nodeKey"))
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get node key from store", "err", err)
	}

	localEccKeyRaw, err := k.ConfigStore().Get([]byte("localEccKey"))
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get local ecc key from store", "err", err)
	}

	serial, err := k.HostDataStore().Get(hostDataKeySerial)
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get serial from store", "err", err)
	}

	hardwareUuid, err := k.HostDataStore().Get(hostDataKeyHardwareUuid)
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get hardware uuid from store", "err", err)
	}

	munemo, err := k.HostDataStore().Get(hostDataKeyMunemo)
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get munemo from store", "err", err)
	}

	deviceId, err := k.ServerProvidedDataStore().Get([]byte("device_id"))
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get device id from store", "err", err)
	}

	remoteIp, err := k.ServerProvidedDataStore().Get([]byte("remote_ip"))
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get remote ip from store", "err", err)
	}

	tombstoneId, err := k.ServerProvidedDataStore().Get([]byte("tombstone_id"))
	if err != nil {
		k.Slogger().Log(context.TODO(), slog.LevelWarn, "could not get tombstone id from store", "err", err)
	}

	dataToStore := oldHostData{
		NodeKey:        string(nodeKey),
		LocalEccKey:    string(localEccKeyRaw),
		Serial:         string(serial),
		HardwareUUID:   string(hardwareUuid),
		Munemo:         string(munemo),
		DeviceID:       string(deviceId),
		RemoteIP:       string(remoteIp),
		TombstoneID:    string(tombstoneId),
		ResetTimestamp: time.Now().Unix(),
	}

	previousHostData, err := k.HostDataStore().Get(hostDataKeyOldHostData)
	if err != nil {
		return fmt.Errorf("getting previous host data from store: %w", err)
	}

	var hostDataCollection []oldHostData
	if previousHostData == nil {
		// No previous database resets
		hostDataCollection = []oldHostData{dataToStore}
	} else {
		if err := json.Unmarshal(previousHostData, &hostDataCollection); err != nil {
			return fmt.Errorf("unmarshalling previous host data: %w", err)
		}
		hostDataCollection = append(hostDataCollection, dataToStore)
	}

	hostDataCollectionRaw, err := json.Marshal(hostDataCollection)
	if err != nil {
		return fmt.Errorf("marshalling host data for storage: %w", err)
	}

	if err := k.HostDataStore().Set(hostDataKeyOldHostData, hostDataCollectionRaw); err != nil {
		return fmt.Errorf("storing old host data: %w", err)
	}

	return nil
}

// wipeDatabase iterates over all stores in the database, deleting all keys from
// each one.
func wipeDatabase(k types.Knapsack) {
	for storeName, store := range k.Stores() {
		keysToDelete := make([][]byte, 0)
		if err := store.ForEach(func(k []byte, _ []byte) error {
			// Preserve the backup data
			if storeName == storage.HostDataStore && string(k) == string(hostDataKeyOldHostData) {
				return nil
			}

			keysToDelete = append(keysToDelete, k)
			return nil
		}); err != nil {
			k.Slogger().Log(context.TODO(), slog.LevelWarn, "iterating over keys in store", "store_name", storeName, "err", err)
			continue
		}

		if err := store.Delete(keysToDelete...); err != nil {
			k.Slogger().Log(context.TODO(), slog.LevelWarn, "deleting keys in store", "store_name", storeName, "err", err)
		}
	}
}
