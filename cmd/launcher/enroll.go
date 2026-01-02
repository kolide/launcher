package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/kolide/launcher/ee/agent/listener"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
)

func runEnroll(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	// Enroll assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	// Parse flags
	var (
		flagset           = flag.NewFlagSet("enroll", flag.ExitOnError)
		flEnrollmentToken = flagset.String("enrollment_token", "", "Enrollment token")
		flConfigFilePath  = flagset.String("config", launcher.DefaultConfigFilePath, "config file to parse options from (optional)")
	)
	if err := ff.Parse(flagset, args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}
	opts, err := launcher.ParseOptions("enroll", []string{"-config", *flConfigFilePath})
	if err != nil {
		return fmt.Errorf("parsing options for subcommand enroll: %w", err)
	}
	if *flEnrollmentToken == "" {
		return errors.New("enrollment_token is required")
	}

	// Set up logging to stdout
	slogLevel := slog.LevelInfo
	if opts.Debug {
		slogLevel = slog.LevelDebug
	}
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	// Set up connection to root launcher process
	clientConn, err := listener.NewLauncherClientConnection(opts.RootDirectory, listener.RootLauncherListenerSocketPrefix)
	if err != nil {
		return fmt.Errorf("opening listener: %w", err)
	}
	defer clientConn.Close()

	// Send over the enrollment token
	if err := clientConn.Enroll(*flEnrollmentToken); err != nil {
		return fmt.Errorf("performing enrollment: %w", err)
	}

	systemMultiSlogger.Log(context.Background(), slog.LevelInfo,
		"successfully completed enrollment",
	)

	return nil
}
