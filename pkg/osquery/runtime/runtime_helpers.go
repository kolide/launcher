//go:build !windows
// +build !windows

package runtime

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// kill process group kills a process and all its children.
func killProcessGroup(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process group %d: %w", cmd.Process.Pid, err)
	}
	return nil
}

func SocketPath(rootDir string) string {
	return filepath.Join(rootDir, "osquery.sock")
}

func platformArgs() []string {
	return nil
}

func isExitOk(err error) bool {
	return false
}
