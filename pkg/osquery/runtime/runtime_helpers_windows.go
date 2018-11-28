// +build windows

package runtime

import (
	"os/exec"
	"syscall"
)

const extensionName = `osquery-extension.exe`

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func killProcessGroup(cmd *exec.Cmd) error {
	// TODO: implement
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
