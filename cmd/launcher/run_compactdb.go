package main

import (
	"errors"
	"path/filepath"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/launcher"
)

func runCompactDb(args []string) error {
	opts, err := launcher.ParseOptions("compactdb", args)
	if err != nil {
		return err
	}

	if opts.RootDirectory == "" {
		return errors.New("No root directory specified")
	}

	// relevel
	logger := logutil.NewServerLogger(opts.Debug)

	boltPath := filepath.Join(opts.RootDirectory, "launcher.db")

	oldDbPath, err := agent.DbCompact(boltPath, opts.CompactDbMaxTx)
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "Done compacting. Safe to remove old db", "path", oldDbPath)

	return nil
}
