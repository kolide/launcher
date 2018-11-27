// +build windows

package runtime

import (
	"os/exec"
	"syscall"
)

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func killProcessGroup(cmd *exec.Cmd) error {
	// TODO: implement
	return nil
}
