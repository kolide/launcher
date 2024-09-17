package uninstall

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/kolide/launcher/ee/agent"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/types"
)

const (
	resetReasonUninstallRequested = "remote uninstall requested"
)

// Uninstall just removes the enroll secret file and wipes the database.
// Logs errors, but does not return them, because we want to try each step independently.
// If exitOnCompletion is true, it will also disable launcher autostart and exit.
func Uninstall(ctx context.Context, k types.Knapsack, exitOnCompletion bool) {
	slogger := k.Slogger().With("component", "uninstall")

	if err := removeEnrollSecretFile(k); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"removing enroll secret file",
			"err", err,
		)
	}

	if err := agent.ResetDatabase(ctx, k, resetReasonUninstallRequested); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"resetting database",
			"err", err,
		)
	}

	backupDbPaths := agentbbolt.BackupLauncherDbLocations(k.RootDirectory())
	for _, db := range backupDbPaths {
		if err := os.Remove(db); err != nil {
			slogger.Log(ctx, slog.LevelError,
				"removing backup database",
				"err", err,
			)
		}
	}

	if !exitOnCompletion {
		return
	}

	if err := disableAutoStart(ctx, k); err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"disabling auto start",
			"err", err,
		)
	}

	os.Exit(0) //nolint:forbidigo // Since we're disabling launcher, it is probably fine to call os.Exit here and skip a graceful shutdown
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
