package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/shipper"
	"github.com/kolide/launcher/pkg/launcher"
)

// sudo /usr/local/kolide-k2/bin/launcher flareupload "note" --debug_upload_request_url="https://example.com"
func runFlareUpload(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	note := ""
	if len(args) > 0 {
		note = args[0]
		args = args[1:]
	}

	opts, err := launcher.ParseOptions("flareupload", args)
	if err != nil {
		return err
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, inmemory.NewStore(logger), fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	ctx := context.Background()
	shipper, err := shipper.New(logger, k, shipper.WithNote(note))
	if err != nil {
		return err
	}
	return checkups.RunFlare(ctx, k, shipper, checkups.StandaloneEnviroment)
}

// sudo /usr/local/kolide-k2/bin/launcher flare
func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	opts, err := launcher.ParseOptions("flare", args)
	if err != nil {
		return err
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, inmemory.NewStore(logger), fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	ctx := context.Background()

	var (
		dirPath = env.String("KOLIDE_AGENT_FLARE_ZIP_DIR_PATH", "")
	)

	id := ulid.New()
	reportName := fmt.Sprintf("kolide_agent_flare_report_%s", id)
	reportPath := fmt.Sprintf("%s.zip", filepath.Join(dirPath, reportName))

	flare, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("creating flare file (%s): %w", reportPath, err)
	}
	defer flare.Close()

	return checkups.RunFlare(ctx, k, flare, checkups.StandaloneEnviroment)
}
