// +build !windows

package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/go-kit/kit/log/level"
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

func ensureProperPermissions(o *OsqueryInstance, path string) error {
	fd, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "stat-ing path")
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
		return errors.Wrap(err, "attempting to chown path")
	}
	return nil
}
