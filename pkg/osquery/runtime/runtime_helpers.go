//go:build !windows

package runtime

import (
	"fmt"
	"path/filepath"
	"syscall"
)

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// kill process group kills a process and all its children.
func killProcessGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process group %d: %w", pid, err)
	}
	return nil
}

func SocketPath(rootDir string, id string) string {
	return filepath.Join(rootDir, fmt.Sprintf("osquery-%s.sock", id))
}

func platformArgs() []string {
	return nil
}

func isExitOk(_ error) bool {
	return false
}
