// +build !windows

package autoupdate

import (
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/updater/tuf"
)

// handler is called by the tuf package when tuf detects a change with
// the remote metadata.
// The handler method will do the following:
// 1) untar the staged staged file,
// 2) replace the existing binary,
// 3) call the Updater's finalizer method, usually a restart function for the running binary.
func (u *Updater) handler() tuf.NotificationHandler {
	return func(stagingPath string, err error) {
		level.Debug(u.logger).Log("msg", "new staged tuf file", "file", stagingPath, "target", u.target, "binary", u.destination)

		if err != nil {
			level.Info(u.logger).Log("msg", "download failed", "target", u.target, "err", err)
			return
		}

		if err := fs.UntarBundle(stagingPath, stagingPath); err != nil {
			level.Info(u.logger).Log("msg", "untar downloaded target", "binary", u.target, "err", err)
			return
		}

		binary := filepath.Join(filepath.Dir(stagingPath), filepath.Base(u.destination))
		if err := os.Rename(binary, u.destination); err != nil {
			level.Info(u.logger).Log("msg", "update binary from staging dir", "binary", u.destination, "err", err)
			return
		}

		if err := os.Chmod(u.destination, 0755); err != nil {
			level.Info(u.logger).Log("msg", "setting +x permissions on binary", "binary", u.destination, "err", err)
			return
		}

		if err := u.finalizer(); err != nil {
			level.Info(u.logger).Log("msg", "calling restart function for updated binary", "binary", u.destination, "err", err)
			return
		}

		level.Debug(u.logger).Log("msg", "completed update for binary", "binary", u.destination)
	}
}
