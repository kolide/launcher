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
)

func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	defaultAutoupdate = true
	setDefaultPaths()

	opts, err := parseOptions("flare", args)
	if err != nil {
		return err
	}

	var (
		//flHostname = flag.String("hostname", "dababe.launcher.kolide.com:443", "")

		// not documented via flags on purpose
		//enrollSecret      = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "flare_ping")
		//serverURL         = env.String("KOLIDE_LAUNCHER_HOSTNAME", *flHostname)
		//insecureTLS       = env.Bool("KOLIDE_LAUNCHER_INSECURE", false)
		//insecureTransport = env.Bool("KOLIDE_LAUNCHER_INSECURE_TRANSPORT", false)
		//flareSocketPath   = env.String("FLARE_SOCKET_PATH", agent.TempPath("flare.sock"))
		dirPath = env.String("KOLIDE_AGENT_FLARE_ZIP_DIR_PATH", "")
		//upload  = env.Bool("KOLIDE_AGENT_FLARE_UPLOAD", false)

		//certPins [][]byte
		//rootPool *x509.CertPool
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
	checkups.RunFlare(ctx, k, flare)

	return nil
}
