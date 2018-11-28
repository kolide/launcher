// +build windows

package runtime

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/pkg/errors"
)

const extensionName = `osquery-extension.exe`

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

func socketPath(rootDir string) string {
	return `\\.\pipe\kolide.em`
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
