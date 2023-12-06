package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	"go.etcd.io/bbolt"
)

var (
	hostDataKeySerial       = []byte("serial")
	hostDataKeyHardwareUuid = []byte("hardware_uuid")
	hostDataKeySecret       = []byte("secret")
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

	enrollSecretChanged := false
	currentEnrollSecret, err := currentSecret(k)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelWarn, "could not get current enroll secret", "err", err)
	} else {
		enrollSecretChanged = valueChanged(k, currentEnrollSecret, hostDataKeySecret)
	}

	if serialChanged || hardwareUUIDChanged || enrollSecretChanged {
		k.Slogger().Log(ctx, slog.LevelWarn, "detected new hardware or rollout, backing up and resetting database",
			"serial_changed", serialChanged,
			"hardware_uuid_changed", hardwareUUIDChanged,
			"enroll_secret_changed", enrollSecretChanged)

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
		if err := k.HostDataStore().Set(hostDataKeySecret, []byte(currentEnrollSecret)); err != nil {
			k.Slogger().Log(ctx, slog.LevelWarn, "could not set secret in host data store", "err", err)
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

	osqProc, err := runsimple.NewOsqueryProcess(osqPath, runsimple.WithStdout(&respBytes))
	if err != nil {
		return "", "", fmt.Errorf("could not create osquery process to determine hardware UUID or serial: %w", err)
	}

	osqCtx, osqCancel := context.WithTimeout(ctx, 10*time.Second)
	defer osqCancel()

	if sqlErr := osqProc.RunSql(osqCtx, []byte(query)); osqCtx.Err() != nil {
		return "", "", fmt.Errorf("querying hardware UUID and serial returned ctx error: %w", osqCtx.Err())
	} else if sqlErr != nil {
		return "", "", fmt.Errorf("querying hardware UUID and serial returned error: %w", sqlErr)
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

// currentSecret retrieves the enrollment secret from either the knapsack or the filesystem,
// depending on launcher configuration.
func currentSecret(k types.Knapsack) (string, error) {
	// Do we want to extract (and store) the tenant munemo, instead of storing the enrollment
	// secret?
	if k.EnrollSecret() != "" {
		return k.EnrollSecret(), nil
	}

	if k.EnrollSecretPath() != "" {
		content, err := os.ReadFile(k.EnrollSecretPath())
		if err == nil {
			return string(bytes.TrimSpace(content)), nil
		}

		return "", fmt.Errorf("could not read secret at enroll_secret_path %s: %w", k.EnrollSecretPath(), err)
	}

	return "", errors.New("enroll secret and secret path both unset")
}

// takeDatabaseBackup takes a backup of the current database and compresses it, storing
// it in the root directory.
func takeDatabaseBackup(k types.Knapsack) error {
	backupFilepath := filepath.Join(k.RootDirectory(), "launcher.db.bak.zip")
	f, err := os.Create(backupFilepath)
	if err != nil {
		return fmt.Errorf("creating backup database file: %w", err)
	}
	defer f.Close()

	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	backupF, err := zipWriter.Create("launcher.db.bak")
	if err != nil {
		return fmt.Errorf("creating backup database inside zip: %w", err)
	}

	if err := k.BboltDB().View(func(tx *bbolt.Tx) error {
		_, err := tx.WriteTo(backupF)
		return err
	}); err != nil {
		return fmt.Errorf("copying db: %w", err)
	}

	k.Slogger().Log(context.TODO(), slog.LevelInfo, "took database backup", "backup_filepath", backupFilepath)

	return nil
}

// wipeDatabase iterates over all stores in the database, deleting all keys from
// each one.
func wipeDatabase(k types.Knapsack) {
	for storeName, store := range k.Stores() {
		keysToDelete := make([][]byte, 0)
		if err := store.ForEach(func(k []byte, _ []byte) error {
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
