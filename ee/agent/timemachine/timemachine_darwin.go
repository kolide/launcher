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

	// Attempting to run this with a single tmutil call we see a lot of tmutil failures logged with error "argument list too long".
	// To avoid this we run in separate batches.
	exclusionPatternBatches := [][]string{
		// All of our databases -- these are the most important ones to exclude, so we handle them
		// separately in the first batch.
		{
			"*.db*", // expected to match osquery.db, launcher.db, and any launcher.db.bak.X
			"*.sqlite",
		},
		// Handle the rest of the files that aren't ephemeral
		{
			"*.json",
			"augeas-lenses",
			"*.plist",
		},
		// Handle files that may disappear (pid files, desktop process files) separately, so that
		// if we get `Error (100002)` (kPOSIXErrorENOENT) because the file disappeared, we at least
		// won't fail to exclude the more important files above.
		{
			"desktop_*",
			"*.pid",
		},
		// osquery sock/pid files are even more ephemeral and often disappear between globbing for matches
		// and actually finishing running tmutil, so we handle them in their own case here --
		// 1) to reduce the amount of time between globbing and tmutil processing these files,
		// 2) so that any failures here won't disrupt tmutil calls for other, more critical files, and
		// 3) doing these last gives osquery as much time as possible to start up and become stable.
		{
			"osquery*",
		},
	}

	for _, batch := range exclusionPatternBatches {
		addExclusionsFromPathPatterns(ctx, k, batch)
	}
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

	if len(exclusionPaths) == 0 {
		return
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
			"num_exclusions", len(exclusionPaths),
			"first_exclusion", exclusionPaths[0],
		)

		return
	}
}
