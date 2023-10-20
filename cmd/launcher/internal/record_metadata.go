package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/groob/plist"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
)

type (
	// metadataWriter is used as a subscriber for the kolide_server_data
	// subsystem. whenever new data is received, it will rewrite the metadata.json
	// and metadata.plist files to our root install directory
	metadataWriter struct {
		ctx    context.Context
		logger log.Logger
		k      types.Knapsack
	}
	metadata struct {
		DeviceId           string `json:"device_id" plist:"device_id"`
		OrganizationId     string `json:"organization_id" plist:"organization_id"`
		OrganizationMunemo string `json:"organization_munemo" plist:"organization_munemo"`
		Timestamp          string `json:"timestamp" plist:"timestamp"`
		Version            string `json:"version" plist:"version"`
	}
)

func NewMetadataWriter(ctx context.Context, logger log.Logger, k types.Knapsack) *metadataWriter {
	return &metadataWriter{
		ctx:    ctx,
		logger: logger,
		k:      k,
	}
}

func (mw *metadataWriter) Ping() {
	metadata := newMetadataTemplate()
	if err := mw.populateLatestServerData(metadata); err != nil {
		level.Debug(mw.logger).Log("msg", "unable to collect latest server data, metadata files will be incomplete", "err", err)
	}

	if err := mw.recordMetadata(metadata); err != nil {
		level.Error(mw.logger).Log("msg", "unable to write out metadata files", "err", err)
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

	deviceId, err := store.Get([]byte("device_id"))
	if err != nil {
		return err
	} else if string(deviceId) == "" {
		return errors.New("device_id is not yet present in ServerProvidedDataStore")
	}

	metadata.DeviceId = string(deviceId)

	organizationId, err := store.Get([]byte("organization_id"))
	if err != nil {
		return err
	} else if string(organizationId) == "" {
		return errors.New("organization_id is not yet present in ServerProvidedDataStore")
	}

	metadata.OrganizationId = string(organizationId)

	munemo, err := store.Get([]byte("munemo"))
	if err != nil {
		return err
	} else if string(munemo) == "" {
		return errors.New("munemo is not yet present in ServerProvidedDataStore")
	}

	metadata.OrganizationMunemo = string(munemo)

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
