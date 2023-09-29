package main

import (
	"context"
	"flag"
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
	"github.com/peterbourgon/ff/v3"
)

// sudo /usr/local/kolide-k2/bin/launcher flareupload "note" --debug_upload_request_url="https://example.com"
func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	var (
		flagset = flag.NewFlagSet("flare", flag.ExitOnError)
		flSave  = flagset.String(
			"save",
			"local",
			"local | upload",
		)
		flNote = flagset.String(
			"note",
			"",
			"note used in URL upload request",
		)
		flUploadRequestURL = flagset.String(
			"upload_request_url",
			"",
			"URL to request a signed upload URL",
		)
	)

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *flSave != "local" && *flSave != "upload" {
		return fmt.Errorf("invalid save option: %s, expected local or upload", *flSave)
	}

	opts, err := launcher.ParseOptions("flareupload", make([]string, 0))
	if err != nil {
		return err
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, inmemory.NewStore(logger), fcOpts...)

	if *flUploadRequestURL != "" {
		flagController.SetDebugUploadRequestURL(*flUploadRequestURL)
	}

	k := knapsack.New(nil, flagController, nil)
	ctx := context.Background()

	if *flSave == "upload" {
		shipper, err := shipper.New(k, shipper.WithNote(*flNote))
		if err != nil {
			return err
		}
		return checkups.RunFlare(ctx, k, shipper, checkups.StandaloneEnviroment)
	}

	// saving flare locally
	var (
		dirPath = env.String("KOLIDE_AGENT_FLARE_ZIP_DIR_PATH", "")
	)

	reportName := fmt.Sprintf("kolide_agent_flare_report_%s", ulid.New())
	reportPath := fmt.Sprintf("%s.zip", filepath.Join(dirPath, reportName))

	flareFile, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("creating flare file (%s): %w", reportPath, err)
	}
	defer flareFile.Close()

	return checkups.RunFlare(ctx, k, flareFile, checkups.StandaloneEnviroment)
}
