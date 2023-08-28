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
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/launcher"
)

func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	// TODO this stuff needs some deeper thinking
	launcher.DefaultAutoupdate = true
	setDefaultPaths()
	_ = launcher.DefaultBinDirectoryPath

	opts, err := launcher.ParseOptions("flare", args)
	if err != nil {
		return err
	}

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

	logger := log.NewLogfmtLogger(os.Stdout)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	ctx := context.Background()
	checkups.RunFlare(ctx, k, flare, checkups.StandaloneEnviroment)

	return nil
}
