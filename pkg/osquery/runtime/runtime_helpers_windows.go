//go:build windows
// +build windows

package runtime

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/kolide/kit/ulid"
)

func setpgid() *syscall.SysProcAttr {
	// TODO: on unix we set the process group id and then
	// terminate that process group.
	return &syscall.SysProcAttr{}
}

func killProcessGroup(cmd *exec.Cmd) error {
	// some discussion here https://github.com/golang/dep/pull/857
	// TODO: should we check err?
	exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
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
