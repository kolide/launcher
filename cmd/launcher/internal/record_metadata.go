package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/groob/plist"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
)

type metadata struct {
	DeviceId       string `json:"device_id" plist:"device_id"`
	OrganizationId string `json:"organization_id" plist:"organization_id"`
	Timestamp      string `json:"timestamp" plist:"timestamp"`
	Version        string `json:"version" plist:"version"`
}

func RecordMetadata(rootDir string, ctx context.Context, knapsack types.Knapsack) error {
	metadataJSONFile := filepath.Join(rootDir, "metadata.json")
	sdc := checkups.NewServerDataCheckup(knapsack)
	if err := sdc.Run(ctx, io.Discard); err != nil {
		return fmt.Errorf("unable to gather metadata, error: %w", err)
	}

	metadata := metadata{
		DeviceId:       sdc.DeviceId,
		OrganizationId: sdc.OrganizationId,
		Timestamp:      time.Now().String(),
		Version:        version.Version().Version,
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to JSON marshal metadata, error: %w", err)
	}

	err = os.WriteFile(metadataJSONFile, metadataJSON, 0644)
	if err != nil {
		return fmt.Errorf("unable to write JSON metadata, error: %w", err)
	}

	if runtime.GOOS != "darwin" {
		return nil
	}

	metadataPlistFile := filepath.Join(rootDir, "metadata.plist")
	metadataPlist, err := plist.MarshalIndent(metadata, "  ")

	if err != nil {
		return fmt.Errorf("unable to Plist marshal metadata, error: %w", err)
	}

	err = os.WriteFile(metadataPlistFile, metadataPlist, 0644)
	if err != nil {
		return fmt.Errorf("unable to write plist metadata, error: %w", err)
	}

	return nil
}
