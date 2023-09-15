package main

import (
	"bytes"
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
	shipping "github.com/kolide/launcher/pkg/debug/shipper"
	"github.com/kolide/launcher/pkg/launcher"
)

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
	flagController := flags.NewFlagController(logger, inmemory.NewStore(logger), fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	ctx := context.Background()

	k.SetDebugUploadRequestURL(env.String("KOLIDE_AGENT_FLARE_REQUEST_URL", ""))
	if k.DebugUploadRequestURL() != "" {
		flareWriter := &bytes.Buffer{}
		if err := checkups.RunFlare(ctx, k, flareWriter, checkups.StandaloneEnviroment); err != nil {
			return err
		}

		return shipping.Ship(logger, k, flareWriter)
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
