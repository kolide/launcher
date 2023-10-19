package internal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/groob/plist"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/debug/checkups"
)

type metadata struct {
	DeviceId       string `json:"device_id" plist:"device_id"`
	OrganizationId string `json:"organization_id" plist:"organization_id"`
	Timestamp      string `json:"timestamp" plist:"timestamp"`
	Version        string `json:"version" plist:"version"`
}

// RecordMetadata writes out both a json and plist (for darwin) file including all information
// in the metadata struct to the root install directory
func RecordMetadata(ctx context.Context, logger log.Logger, knapsack types.Knapsack) {
	metadataJSONFile := filepath.Join(knapsack.RootDirectory(), "metadata.json")
	metadata := metadata{
		DeviceId:       "",
		OrganizationId: "",
		Timestamp:      time.Now().String(),
		Version:        version.Version().Version,
	}

	if err := backoff.WaitFor(func() error {
		sdc := checkups.NewServerDataCheckup(knapsack)
		if err := sdc.Run(ctx, io.Discard); err != nil {
			return err
		}

		if sdc.DeviceId == "" {
			return errors.New("unable to gather device_id from server data")
		}

		metadata.DeviceId = sdc.DeviceId
		metadata.OrganizationId = sdc.OrganizationId

		return nil
	}, 10*time.Minute, 1*time.Minute); err != nil {
		level.Error(logger).Log(
			"msg", "unable to gather device metadata within timeout, metadata files will be incomplete",
			"err", err,
		)
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		level.Error(logger).Log("msg", "unable to JSON marshal metadata", "err", err)
		return
	}

	if err = os.WriteFile(metadataJSONFile, metadataJSON, 0644); err != nil {
		level.Error(logger).Log("msg", "unable to write JSON metadata", "err", err)
		return
	}

	if runtime.GOOS != "darwin" {
		return
	}

	metadataPlistFile := filepath.Join(knapsack.RootDirectory(), "metadata.plist")
	metadataPlist, err := plist.MarshalIndent(metadata, "  ")

	if err != nil {
		level.Error(logger).Log("msg", "unable to plist marshal metadata", "err", err)
		return
	}

	if err = os.WriteFile(metadataPlistFile, metadataPlist, 0644); err != nil {
		level.Error(logger).Log("msg", "unable to write plist metadata", "err", err)
	}
}
