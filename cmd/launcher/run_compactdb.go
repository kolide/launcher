package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
)

func runCompactDb(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	var (
		flagset = flag.NewFlagSet("launcher compactdb", flag.ExitOnError)
		// Flags specific to this subcommand
		flDbFileName     = flagset.String("db", "launcher.db", "The launcher.db or launcher.db.bak file to be compacted")
		flCompactDbMaxTx = flagset.Int64("compactdb-max-tx", 65536, "Maximum transaction size used when compacting the internal DB")
		// Flags shared by runLauncher/other subcommands, to be parsed by launcher.ParseOptions
		flRootDirectory  = flagset.String("root_directory", "", "The location of the local database, pidfiles, etc.")
		flConfigFilePath = flagset.String("config", "", "config file to parse options from (optional)")
	)
	flagset.Usage = commandUsage(flagset, "launcher compactdb")
	if err := ff.Parse(flagset, args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	launcherOptions := make([]string, 0)
	if *flRootDirectory != "" {
		launcherOptions = append(launcherOptions, "-root_directory", *flRootDirectory)
	}
	if *flConfigFilePath != "" {
		launcherOptions = append(launcherOptions, "-config", *flConfigFilePath)
	}

	opts, err := launcher.ParseOptions("compactdb", launcherOptions)
	if err != nil {
		return fmt.Errorf("parsing launcher options: %w", err)
	}
	if opts.RootDirectory == "" {
		return errors.New("no root directory specified")
	}

	// Add handler to write to stdout
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))

	boltPath := filepath.Join(opts.RootDirectory, "launcher.db")
	if *flDbFileName != "" {
		// Check to make sure this is a launcher.db or launcher.db.bak file
		givenFile := filepath.Base(*flDbFileName)
		if givenFile != "launcher.db" && !strings.HasPrefix(givenFile, "launcher.db.bak") {
			return fmt.Errorf("invalid db file given, must be launcher.db or a backup: %s", *flDbFileName)
		}

		boltPath = filepath.Join(opts.RootDirectory, givenFile)

		// If a directory was provided, make sure it matches the root directory
		if givenFile != *flDbFileName && boltPath != *flDbFileName {
			return fmt.Errorf("db file %s is not in root directory (expected %s)", *flDbFileName, boltPath)
		}
	}

	systemMultiSlogger.Log(context.TODO(), slog.LevelInfo,
		"preparing to compact db",
		"path", boltPath,
	)

	oldDbPath, err := agent.DbCompact(boltPath, *flCompactDbMaxTx)
	if err != nil {
		return err
	}

	systemMultiSlogger.Log(context.TODO(), slog.LevelInfo,
		"done compacting, safe to remove old db",
		"path", oldDbPath,
	)

	return nil
}
