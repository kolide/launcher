//go:build darwin
// +build darwin

package timemachine

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
)

// ExcludeLauncherDB adds the launcher db to the time machine exclusions for
// darwin and is noop for other oses
func ExcludeLauncherDB(ctx context.Context, k types.Knapsack) {
	dbPath := k.BboltDB().Path()
	cmd, err := allowedcmd.Tmutil(ctx, "addexclusion", dbPath)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"failed to add launcher db to time machine exclusions",
			"err", err,
			"path", dbPath,
		)
		return
	}

	if err := cmd.Run(); err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"failed to add launcher db to time machine exclusions",
			"err", err,
			"path", dbPath,
		)
		return
	}
}
