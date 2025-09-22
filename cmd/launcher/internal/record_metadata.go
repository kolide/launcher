package internal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/groob/plist"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/types"
)

type (
	// metadataWriter is used as a subscriber for the kolide_server_data
	// subsystem. whenever new data is received, it will rewrite the metadata.json
	// and metadata.plist files to our root install directory
	metadataWriter struct {
		slogger *slog.Logger
		k       types.Knapsack
	}
	metadata struct {
		DeviceId           string `json:"device_id" plist:"device_id"`
		OrganizationId     string `json:"organization_id" plist:"organization_id"`
		OrganizationMunemo string `json:"organization_munemo" plist:"organization_munemo"`
		Timestamp          string `json:"timestamp" plist:"timestamp"`
		Version            string `json:"version" plist:"version"`
	}
)

func NewMetadataWriter(slogger *slog.Logger, k types.Knapsack) *metadataWriter {
	return &metadataWriter{
		slogger: slogger.With("component", "metadata_writer"),
		k:       k,
	}
}

func (mw *metadataWriter) Ping() {
	metadata := newMetadataTemplate()
	if err := mw.populateLatestServerData(metadata); err != nil {
		mw.slogger.Log(context.TODO(), slog.LevelDebug,
			"unable to collect latest server data, metadata files will be incomplete",
			"err", err,
		)
	}

	if err := mw.recordMetadata(metadata); err != nil {
		mw.slogger.Log(context.TODO(), slog.LevelDebug,
			"unable to write out metadata files",
			"err", err,
		)
	}
}

func newMetadataTemplate() *metadata {
	return &metadata{
		Timestamp: time.Now().String(),
		Version:   version.Version().Version,
	}
}

func (mw *metadataWriter) populateLatestServerData(metadata *metadata) error {
	store := mw.k.ServerProvidedDataStore()

	if store == nil {
		return errors.New("ServerProvidedDataStore is uninitialized")
	}

	metadata.DeviceId = mw.getServerDataValue(store, "device_id")
	metadata.OrganizationId = mw.getServerDataValue(store, "organization_id")
	metadata.OrganizationMunemo = mw.getServerDataValue(store, "munemo")

	return nil
}

// recordMetadata writes out both a json and plist (for darwin) file including all information
// in the metadata struct to the root install directory
func (mw *metadataWriter) recordMetadata(metadata *metadata) error {
	metadataJSONFile := filepath.Join(mw.k.RootDirectory(), "metadata.json")
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	if err = os.WriteFile(metadataJSONFile, metadataJSON, 0644); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return nil
	}

	metadataPlistFile := filepath.Join(mw.k.RootDirectory(), "metadata.plist")
	metadataPlist, err := plist.MarshalIndent(metadata, "  ")

	if err != nil {
		return err
	}

	if err = os.WriteFile(metadataPlistFile, metadataPlist, 0644); err != nil {
		return err
	}

	return nil
}

func (mw *metadataWriter) getServerDataValue(store types.KVStore, key string) string {
	val, err := store.Get([]byte(key))
	if err != nil {
		mw.slogger.Log(context.TODO(), slog.LevelDebug,
			"unable to collect value for key from server_data, will re-attempt on next update",
			"key", key,
			"err", err,
		)

		return ""
	}

	if string(val) == "" {
		mw.slogger.Log(context.TODO(), slog.LevelDebug,
			"server_data was missing value for key, will re-attempt on next update",
			"key", key,
			"err", err,
		)

		return ""
	}

	return string(val)
}
