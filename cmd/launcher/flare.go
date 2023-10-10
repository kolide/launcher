package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/shipper"
	"github.com/kolide/launcher/pkg/launcher"
)

// runFlare is a command that runs the flare checkup and saves the results locally or uploads them to a server.
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
		flOutputDir = flagset.String(
			"output_dir",
			".",
			"path to directory to save flare output",
		)
		flUploadRequestURL = flagset.String(
			"upload_request_url",
			"",
			"URL to request a signed upload URL",
		)
	)

	if err := flagset.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *flSave != "local" && *flSave != "upload" {
		return fmt.Errorf("invalid save option: %s, expected local or upload", *flSave)
	}

	// were passing an empty array here just to get the default options
	opts, err := launcher.ParseOptions("flareupload", make([]string, 0))
	if err != nil {
		return err
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, inmemory.NewStore(logger), fcOpts...)

	k := knapsack.New(nil, flagController, nil)
	ctx := context.Background()

	if *flSave == "upload" {
		shipper, err := shipper.New(k, shipper.WithNote(strings.Join(flagset.Args(), " ")), shipper.WithUploadRequestURL(*flUploadRequestURL))
		if err != nil {
			return err
		}
		return checkups.RunFlare(ctx, k, shipper, checkups.StandaloneEnviroment)
	}

	// saving flare locally
	reportName := fmt.Sprintf("kolide_agent_flare_report_%s", ulid.New())
	reportPath := fmt.Sprintf("%s.zip", filepath.Join(*flOutputDir, reportName))

	flareFile, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("creating flare file (%s): %w", reportPath, err)
	}
	defer flareFile.Close()

	return checkups.RunFlare(ctx, k, flareFile, checkups.StandaloneEnviroment)
}
