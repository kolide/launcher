package uninstall

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/kolide/launcher/ee/agent/types"
)

func Uninstall(ctx context.Context, k types.Knapsack) {
	uninstallNoExit(ctx, k)

	if err := disableAutoStart(ctx); err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"disabling auto start",
			"err", err,
		)
	}

	// TODO: remove start up files
	// TODO: remove installation

	os.Exit(0)
}

func uninstallNoExit(ctx context.Context, k types.Knapsack) {
	slogger := k.Slogger().With("component", "uninstall")

	if err := removeEnrollSecretFile(k); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"removing enroll secret file",
			"err", err,
		)
	}

	if err := removeDatabase(k); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"removing database",
			"err", err,
		)
	}
}

func removeEnrollSecretFile(knapsack types.Knapsack) error {
	if knapsack.EnrollSecretPath() == "" {
		return errors.New("no enroll secret path set")
	}

	if err := os.Remove(knapsack.EnrollSecretPath()); err != nil {
		return err
	}

	return nil
}

func removeDatabase(k types.Knapsack) error {
	path := k.BboltDB().Path()

	if err := k.BboltDB().Close(); err != nil {
		return fmt.Errorf("closing bbolt db: %w", err)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("deleting bbolt db: %w", err)
	}

	return nil
}
