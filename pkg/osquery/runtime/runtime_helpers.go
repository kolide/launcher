//go:build !windows
// +build !windows

package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/go-kit/kit/log/level"
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

func ensureProperPermissions(o *OsqueryInstance, path string) error {
	fd, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat-ing path: %w", err)
	}
	sys := fd.Sys().(*syscall.Stat_t)
	isRootOwned := (sys.Uid == 0)
	isProcOwned := (sys.Uid == uint32(os.Geteuid()))

	if isRootOwned || isProcOwned {
		return nil
	}

	level.Info(o.logger).Log(
		"msg", "unsafe permissions detected on path",
		"path", path,
	)

	// chown the path. This could potentially be insecure, since
	// we're basically chown-ing whatever is there to root, but a certain
	// level of privilege is needed to place something in the launcher root
	// directory.
	if err = os.Chown(path, os.Getuid(), os.Getgid()); err != nil {
		return fmt.Errorf("attempting to chown path: %w", err)
	}
	return nil
}
