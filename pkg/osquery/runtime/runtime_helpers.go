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

func ensureProperPermissions(o *OsqueryInstance, paths []string) error {
	for _, p := range paths {
		fd, err := os.Stat(p)
		if err != nil {
			return errors.Wrap(err, "stat-ing extension path")
		}
		sys := fd.Sys().(*syscall.Stat_t)
		isRootOwned := (sys.Uid == 0)
		isProcOwned := (sys.Uid == uint32(os.Geteuid()))

		if !(isRootOwned || isProcOwned) {
			level.Info(o.logger).Log(
				"msg", "unsafe permissions detected for file",
				"path", p)

			// chown the provided file. This could potentially be insecure,
			// since we're basically chown-ing whatever is there to root, but a
			// certain level of privilege is needed to place something in the
			// launcher root directory.
			err := os.Chown(p, os.Getuid(), os.Getgid())
			if err != nil {
				return errors.Wrapf(err, "attempting to chown file %s", p)
			}
		}

	}
	return nil
}
