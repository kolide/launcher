//go:build darwin
// +build darwin

package timemachine

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
)

// AddExclusions adds specific launcher time machine exclusions for
// darwin and is noop for other oses
func AddExclusions(ctx context.Context, k types.Knapsack) {
	/*
		example of root dir:
		augeas-lenses/                         menu.json
		debug-2024-01-09T21-15-14.055.json.gz  menu_template.json
		debug-2024-01-09T21-20-54.621.json.gz  metadata.json
		debug-2024-01-09T21-23-58.097.json.gz  metadata.plist
		debug-2024-01-10T18-49-33.610.json.gz  osquery.autoload
		debug.json                             osquery.db/
		desktop_501/                           osquery.pid
		desktop_503/                           osquery.sock
		kolide.png                             osquery.sock.51807
		kv.sqlite                              osquery.sock.63071
		launcher-tuf/                          osqueryd-tuf/
		launcher-version-1.4.1-4-gdb7106f      tuf/
		launcher.db                            updates/
		launcher.pid
	*/

	exclusionPatternsFirstBatch := []string{
		"*.json",
		"*.db*", // expected to match osquery.db, launcher.db, and any launcher.db.bak.X
	}

	addExclusionsFromPathPatterns(ctx, k, exclusionPatternsFirstBatch)

	// Attempting to run this with a single tmutil call we see a lot of tmutil failures logged with error "argument list too long".
	// To avoid this we run in two separate batches, attempting to cut the post-glob argument list roughly in half
	exclusionPatternsSecondBatch := []string{
		"*.sqlite",
		"desktop_*",
		"*.pid",
		"augeas-lenses",
		"*.plist",
		"osquery*",
	}

	addExclusionsFromPathPatterns(ctx, k, exclusionPatternsSecondBatch)
}

func addExclusionsFromPathPatterns(ctx context.Context, k types.Knapsack, exclusionPatterns []string) {
	var exclusionPaths []string
	for _, pattern := range exclusionPatterns {
		matches, err := filepath.Glob(filepath.Join(k.RootDirectory(), pattern))

		if err != nil {
			k.Slogger().Log(ctx, slog.LevelError,
				"could not create glob for launcher machine exclusions",
				"err", err,
			)

			continue
		}

		exclusionPaths = append(exclusionPaths, matches...)
	}

	cmd, err := allowedcmd.Tmutil(ctx, append([]string{"addexclusion"}, exclusionPaths...)...)
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"could not create tmutil command to add launcher machine exclusions",
			"err", err,
		)
		return
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		k.Slogger().Log(ctx, slog.LevelError,
			"running command to add launcher machine exclusions",
			"err", err,
			"output", string(out),
		)

		return
	}
}
