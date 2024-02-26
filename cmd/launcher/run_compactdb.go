package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runCompactDb(args []string) error {
	opts, err := launcher.ParseOptions("compactdb", args)
	if err != nil {
		return err
	}

	if opts.RootDirectory == "" {
		return errors.New("no root directory specified")
	}

	// relevel
	slogLevel := slog.LevelInfo
	if opts.Debug {
		slogLevel = slog.LevelDebug
	}

	slogger := multislogger.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	})).Logger

	boltPath := filepath.Join(opts.RootDirectory, "launcher.db")

	oldDbPath, err := agent.DbCompact(boltPath, opts.CompactDbMaxTx)
	if err != nil {
		return err
	}

	slogger.Log(context.TODO(), slog.LevelInfo,
		"done compacting, safe to remove old db",
		"path", oldDbPath,
	)

	return nil
}
