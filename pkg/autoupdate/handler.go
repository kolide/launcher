package autoupdate

import (
	"context"
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
// 1) untar the staged download
// 2) place binary into the updates/<timestamp> directory
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
				"msg", "making updates directory",
				"dir", updateDir,
				"err", err)
			return
		}

		cleanupBrokenUpdate := func() {
			if err := os.RemoveAll(updateDir); err != nil {
				level.Error(u.logger).Log(
					"msg", "failed to removed broken update directory",
					"updateDir", updateDir,
					"err", err,
				)
			}
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
			cleanupBrokenUpdate()
			return
		}

		// Ensure it's executable
		if err := os.Chmod(outputBinary, 0755); err != nil {
			level.Error(u.logger).Log(
				"msg", "setting +x permissions on binary",
				"binary", outputBinary,
				"err", err,
			)
			cleanupBrokenUpdate()
			return
		}

		// Check that it all came through okay
		if err := checkExecutable(context.TODO(), outputBinary, "--version"); err != nil {
			level.Error(u.logger).Log(
				"msg", "Broken updated binary. Removing",
				"target", u.target,
				"outputBinary", outputBinary,
				"err", err,
			)
			cleanupBrokenUpdate()
			return
		}

		level.Info(u.logger).Log(
			"msg", "Updated Binary ready to go",
			"target", u.target,
			"outputBinary", outputBinary,
		)

		if err := u.finalizer(); err != nil {
			// Some kinds of updates require a full launcher restart. For
			// example, windows doesn't have an exec. Instead launcher exits
			// so the service manager restarts it. There may be others.
			if IsLauncherRestartNeededErr(err) {
				level.Info(u.logger).Log(
					"msg", "signaling for a full restart",
					"binary", outputBinary,
				)
				u.sigChannel <- os.Interrupt
				return
			}

			level.Error(u.logger).Log(
				"msg", "calling restart function for updated binary",
				"binary", outputBinary,
				"err", err)
			// Reaching this point represents an unclear error. Trigger a restart
			u.sigChannel <- os.Interrupt
			return
		}

		level.Debug(u.logger).Log("msg", "completed update for binary", "binary", outputBinary)
	}
}
