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
		flagset          = flag.NewFlagSet("launcher compactdb", flag.ExitOnError)
		flDbFileName     = flagset.String("db", "launcher.db", "The launcher.db or launcher.db.bak file to be compacted")
		flRootDirectory  = flagset.String("root_directory", launcher.DefaultRootDirectoryPath, "The location of the database to be compacted")
		flCompactDbMaxTx = flagset.Int64("compactdb-max-tx", 65536, "Maximum transaction size used when compacting the internal DB")
		flDebug          = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		_                = flagset.String(
			"config",
			"",
			"launcher flags configuration file",
		)
	)

	ffOpts := []ff.Option{
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithIgnoreUndefined(true),
		ff.WithEnvVarNoPrefix(),
	}

	flagset.Usage = commandUsage(flagset, "launcher compactdb")
	if err := ff.Parse(flagset, args, ffOpts...); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *flRootDirectory == "" {
		return errors.New("no root directory specified")
	}

	// relevel
	slogLevel := slog.LevelInfo
	if *flDebug {
		slogLevel = slog.LevelDebug
	}

	// Add handler to write to stdout
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	boltPath := filepath.Join(*flRootDirectory, "launcher.db")
	if *flDbFileName != "" {
		// Check to make sure this is a launcher.db or launcher.db.bak file
		givenFile := filepath.Base(*flDbFileName)
		if givenFile != "launcher.db" && !strings.HasPrefix(givenFile, "launcher.db.bak") {
			return fmt.Errorf("invalid db file given, must be launcher.db or a backup: %s", *flDbFileName)
		}

		boltPath = filepath.Join(*flRootDirectory, givenFile)

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
