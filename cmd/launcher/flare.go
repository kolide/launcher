package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/debug/checkups"
	"github.com/kolide/launcher/ee/debug/shipper"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
)

// runFlare is a command that runs the flare checkup and saves the results locally or uploads them to a server.
func runFlare(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	attachConsole()
	defer detachConsole()

	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	var (
		flagset            = flag.NewFlagSet("flare", flag.ExitOnError)
		flSave             = flagset.String("save", "upload", "local | upload")
		flOutputDir        = flagset.String("output_dir", ".", "path to directory to save flare output")
		flUploadRequestURL = flagset.String("upload_request_url", "https://api.kolide.com/api/agent/flare", "URL to request a signed upload URL")
		flConfigFilePath   = flagset.String("config", launcher.DefaultConfigFilePath, "config file to parse options from (optional)")
	)

	if err := ff.Parse(flagset, args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	opts, err := launcher.ParseOptions("flare", []string{"-config", *flConfigFilePath})
	if err != nil {
		return err
	}

	slogLevel := slog.LevelInfo
	if opts.Debug {
		slogLevel = slog.LevelDebug
	}

	// Add handler to write to stdout
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(systemMultiSlogger.Logger, inmemory.NewStore(), fcOpts...)

	k := knapsack.New(nil, flagController, nil, nil, nil)
	ctx := context.Background()

	type flareDestinationTyp interface {
		io.WriteCloser
		Name() string
	}
	var flareDest flareDestinationTyp
	var successMessage string

	switch *flSave {
	case "upload":
		shipper, err := shipper.New(k, shipper.WithNote(strings.Join(flagset.Args(), " ")), shipper.WithUploadRequestURL(*flUploadRequestURL))
		if err != nil {
			return err
		}
		flareDest = shipper
		successMessage = "flare uploaded successfully"
	case "local":
		reportName := fmt.Sprintf("kolide_agent_flare_report_%s.zip", ulid.New())
		reportPath := filepath.Join(*flOutputDir, reportName)

		flareFile, err := os.Create(reportPath)
		if err != nil {
			return fmt.Errorf("creating flare file (%s): %w", reportPath, err)
		}
		defer flareFile.Close()
		flareDest = flareFile
		successMessage = "flare saved locally"
	default:
		return fmt.Errorf(`invalid save option: %s, expected "local" or "upload"`, *flSave)
	}

	if err := checkups.RunFlare(ctx, k, flareDest, checkups.StandaloneEnviroment); err != nil {
		return err
	}

	systemMultiSlogger.Log(ctx, slog.LevelInfo,
		"flare creation complete",
		"status", successMessage,
		"file", flareDest.Name(),
		"config_path", *flConfigFilePath,
	)

	return nil
}
