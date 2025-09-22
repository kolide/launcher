package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/interactive"
)

func runInteractive(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	opts, err := launcher.ParseOptions("interactive", args)
	if err != nil {
		return err
	}

	// here we are looking for the launcher "proper" root directory so that we know where
	// to find the kv.sqlite where we can pull the auto table construction config from
	if opts.RootDirectory == "" {
		opts.RootDirectory = launcher.DefaultPath(launcher.RootDirectory)
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

	// opts.OsquerydPath will always be set at this point from our default population logic in options.go.
	// but we should attempt to lookup the latest osqueryd binary and use that instead if it exists.
	// we can ignore any errors here and fall back to the path we've already found otherwise
	latestOsquerydBinary, err := tuf.CheckOutLatestWithoutConfig("osqueryd", systemMultiSlogger.Logger)
	if err == nil {
		opts.OsquerydPath = latestOsquerydBinary.Path
	}

	if opts.OsquerydPath == "" {
		return errors.New("could not find osqueryd binary")
	}

	// this is a tmp root directory that launcher can use to store files it needs to run
	// such as the osquery socket and augeas lense files
	interactiveRootDir, err := agent.MkdirTemp("launcher-interactive")
	if err != nil {
		return fmt.Errorf("creating temp dir for interactive mode: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(interactiveRootDir); err != nil {
			fmt.Printf("error removing launcher interactive temp dir: %s\n", err)
		}
	}()

	hasTlsServerCertsOsqueryFlag := false
	// check to see if we were passed a tls_server_certs flag
	for _, v := range opts.OsqueryFlags {
		if strings.HasPrefix(v, "tls_server_certs") {
			hasTlsServerCertsOsqueryFlag = true
			break
		}
	}

	// if we were not passed a tls_server_certs flag, pass default to osquery
	if !hasTlsServerCertsOsqueryFlag {
		certs, err := osquery.InstallCaCerts(interactiveRootDir, systemMultiSlogger.Logger)
		if err != nil {
			return fmt.Errorf("installing CA certs: %w", err)
		}

		opts.OsqueryFlags = append(opts.OsqueryFlags, fmt.Sprintf("tls_server_certs=%s", certs))
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(systemMultiSlogger.Logger, inmemory.NewStore(), fcOpts...)

	knapsack := knapsack.New(nil, flagController, nil, systemMultiSlogger, nil)

	osqueryProc, extensionsServer, err := interactive.StartProcess(knapsack, interactiveRootDir, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("error starting osqueryd: %s", err)
	}
	defer extensionsServer.Shutdown(context.Background())

	// Wait until user exits the shell
	_, err = osqueryProc.Wait()
	if err != nil {
		return fmt.Errorf("error waiting for osqueryd: %s", err)
	}

	return nil
}
