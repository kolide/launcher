package uninstall

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/kolide/launcher/ee/agent"
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

// uninstallNoExit just removes the enroll secret file and wipes the database without
// exiting the process. This is so that we can test a portion of the uninstall process.
func uninstallNoExit(ctx context.Context, k types.Knapsack) {
	slogger := k.Slogger().With("component", "uninstall")

	if err := removeEnrollSecretFile(k); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"removing enroll secret file",
			"err", err,
		)
	}

	agent.WipeDatabase(ctx, k)
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
