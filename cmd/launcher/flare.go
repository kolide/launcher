package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/flareshipping"
	"github.com/kolide/launcher/pkg/launcher"
)

// runFlareInt is an empty struct that implements the RunFlare method so we can pass to flare shipper
type runFlareInt struct{}

func (_ runFlareInt) RunFlare(ctx context.Context, k types.Knapsack, flare io.Writer, environment checkups.RuntimeEnvironmentType) error {
	return checkups.RunFlare(ctx, k, flare, environment)
}

func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()
	_ = launcher.DefaultBinDirectoryPath

	opts, err := launcher.ParseOptions("flare", args)
	if err != nil {
		return err
	}

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	ctx := context.Background()

	var requestUrl = env.String("KOLIDE_AGENT_FLARE_REQUEST_URL", "")
	if requestUrl != "" {
		return flareshipping.RunFlareShip(logger, k, runFlareInt{}, requestUrl)
	}

	// not shipping, write to file
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
	defer func() {
		_ = flare.Close()
	}()

	checkups.RunFlare(ctx, k, flare, checkups.StandaloneEnviroment)

	return nil
}
