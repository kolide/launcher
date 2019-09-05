package autoupdate

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/updater/tuf"
)

// handler is called by the tuf package in two cases. First, and
// confusingly, it's used as an error reporting channel for any kind
// of issue. In this case, it's called with an err set.
//
// Second, it's called when tuf detects a change with the remote metadata.
// The handler method will do the following:
// 1) untar the staged staged file,
// 2) into the updated directory
// 3) call the Updater's finalizer method, usually a restart function for the running binary.
func (u *Updater) handler() tuf.NotificationHandler {
	return func(stagingPath string, err error) {
		if err != nil {
			level.Info(u.logger).Log(
				"msg", "tuf updater returned",
				"target", u.target,
				"err", err)
			return
		}

		level.Debug(u.logger).Log(
			"msg", "Starting to handle a staged TUF download",
			"file", stagingPath,
			"target", u.target,
		)

		// We store the updated file in a dated directory. The
		// dated directory is a bit odd, but it's plastering
		// over how tuf works. This way we ensure we're always
		// running the mostly recently downloaded file.  There
		// are other patterns we should investigate if we
		// change the way we denote stable in notary.
		updateDir := filepath.Join(u.updatesDirectory, strconv.FormatInt(time.Now().Unix(), 10))

		// Note that this is expecting the binary in the
		// tarball to be named binaryName. There some some
		// extension weirdness issues on windows vs posix.
		outputBinary := filepath.Join(updateDir, u.binaryName)

		if err := os.MkdirAll(updateDir, 0755); err != nil {
			level.Error(u.logger).Log(
				"msg", "making updated directory",
				"dir", updateDir,
				"err", err)
			return
		}

		// The UntarBundle(destination, source) paths are a
		// little weird. Source is a tarball, obvious
		// enough. But destination is a string that's passed
		// through filepath.Dir. Which means it strips off the
		// last component.
		if err := fs.UntarBundle(outputBinary, stagingPath); err != nil {
			level.Error(u.logger).Log(
				"msg", "untar downloaded target",
				"binary", outputBinary,
				"err", err,
			)
			return
		}

		// Ensure it's executable
		if err := os.Chmod(outputBinary, 0755); err != nil {
			level.Error(u.logger).Log(
				"msg", "setting +x permissions on binary",
				"binary", outputBinary,
				"err", err,
			)
			return
		}

		level.Info(u.logger).Log(
			"msg", "Updated Binary ready to go",
			"target", u.target,
			"outputBinary", outputBinary,
		)

		if err := u.finalizer(); err != nil {
			level.Error(u.logger).Log(
				"msg", "calling restart function for updated binary",
				"binary", outputBinary,
				"err", err)
			return
		}

		level.Debug(u.logger).Log("msg", "completed update for binary", "binary", outputBinary)
	}
}
