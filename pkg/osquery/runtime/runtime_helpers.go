// +build !windows

package runtime

import (
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
)

const extensionName = `osquery-extension.ext`

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// kill process group kills a process and all its children.
func killProcessGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return errors.Wrapf(err, "kill process group %d", cmd.Process.Pid)
}

func socketPath(rootDir string) string {
	return filepath.Join(rootDir, "osquery.sock")
}

func platformArgs() []string {
	return nil
}

func isExitOk(err error) bool {
	return false
}
