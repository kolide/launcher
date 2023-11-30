//go:build windows
// +build windows

package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/allowedcmd"
	"github.com/pkg/errors"
)

func setpgid() *syscall.SysProcAttr {
	// TODO: on unix we set the process group id and then
	// terminate that process group.
	return &syscall.SysProcAttr{}
}

func killProcessGroup(cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// some discussion here https://github.com/golang/dep/pull/857
	cmd, err := allowedcmd.Taskkill(ctx, "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("running taskkill: output: %s, err: %w", string(out), err)
		}

		if ctx.Err() != nil {
			return fmt.Errorf("running taskkill: context err: %v, err: %w", ctx.Err(), err)
		}

		return fmt.Errorf("running taskkill: err: %w", err)
	}

	return nil
}

func SocketPath(rootDir string) string {
	// On windows, local names pipes paths are all rooted in \\.\pipe\
	// their names are limited to 256 characters, and can include any
	// character other than backslash. They are case insensitive.
	//
	// They have some set of security parameters, which can be set at
	// create time. They are automatically removed when the last handle
	// to pipe is closed.
	//
	// Our usage of the pipe is for shared communication between
	// launcher and osquery. We would like to be able to run multiple
	// launchers.
	//
	// We could use something based on the laumcher root, but given the
	// context this runs in a ulid seems simpler.
	return fmt.Sprintf(`\\.\pipe\kolide-osquery-%s`, ulid.New())
}

func platformArgs() []string {
	return []string{
		"--allow_unsafe",
	}
}

func isExitOk(err error) bool {
	if exiterr, ok := errors.Cause(err).(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			// https://msdn.microsoft.com/en-us/library/cc704588.aspx
			// STATUS_CONTROL_C_EXIT
			return status.ExitStatus() == 3221225786
		}
	}
	return false
}

func ensureProperPermissions(o *OsqueryInstance, path string) error {
	return nil
}
