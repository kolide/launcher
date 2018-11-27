// +build !windows

package runtime

import (
	"os/exec"
	"syscall"

	"github.com/pkg/errors"
)

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// kill process group kills a process and all its children.
func killProcessGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return errors.Wrapf(err, "kill process group %d", cmd.Process.Pid)
}
