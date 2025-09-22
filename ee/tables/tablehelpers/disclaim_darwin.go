//go:build darwin
// +build darwin

package tablehelpers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/kolide/launcher/ee/allowedcmd"
)

// Disclaimed (darwin only) replaces the cmd path and args for use via
// our rundisclaimed subcommand
func Disclaimed(ctx context.Context, disclaimCmdName string) ExecOps {
	return func(cmd *exec.Cmd) error {
		launcherCmd, err := allowedcmd.Launcher(ctx, "rundisclaimed", disclaimCmdName)
		if err != nil {
			return fmt.Errorf("generating launcher command for disclaim: %w", err)
		}

		if len(cmd.Args) < 1 {
			return errors.New("original command for rundisclaimed is missing args")
		}

		// swap the original command path with our own launcher path
		cmd.Path = launcherCmd.Path
		// add any args already specified to the end of "rundisclaimed" <disclaimCmdName>
		// cmd.Args will at least already have the full path to the original target binary in there, omit that
		// so we're left with /path/to/launcher rundisclaimed <disclaimCmdName> [original cmd args...]
		cmd.Args = append(launcherCmd.Args, cmd.Args[1:]...)
		cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE")
		return nil
	}
}
