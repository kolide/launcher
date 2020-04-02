package autoupdate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// This suffix is added to the binary path to find the updates
const updateDirSuffix = "-updates"

type newestSettings struct {
	deleteOld               bool
	deleteCorrupt           bool
	skipFullBinaryPathCheck bool
}

type newestOption func(*newestSettings)

func DeleteOldUpdates() newestOption {
	return func(no *newestSettings) {
		no.deleteOld = true
	}
}

func DeleteCorruptUpdates() newestOption {
	return func(no *newestSettings) {
		no.deleteCorrupt = true
	}
}

// SkipFullBinaryPathCheck skips the final check on FindNewest. This
// is desirable when being called by FindNewestSelf, otherewise we end
// up in a infineite recursion. (The recursion is saved by the exec
// check, but it's better not to trigger it.
func SkipFullBinaryPathCheck() newestOption {
	return func(no *newestSettings) {
		no.skipFullBinaryPathCheck = true
	}
}

// FindNewestSelf invokes `FindNewest` with the running binary path,
// as determined by os.Executable. However, if the current running
// version is the same as the newest on disk, it will return empty string.
func FindNewestSelf(ctx context.Context, opts ...newestOption) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	exPath, err := os.Executable()
	if err != nil {
		return "", errors.Wrap(err, "determine running executable path")
	}

	if exPath == "" {
		return "", errors.New("can't find newest empty string")
	}

	opts = append(opts, SkipFullBinaryPathCheck())

	newest := FindNewest(ctx, exPath, opts...)

	if newest == "" {
		return "", nil
	}

	if exPath == newest {
		return "", nil
	}

	level.Debug(logger).Log(
		"msg", "found an update",
		"newest", newest,
		"exPath", exPath,
	)

	return newest, nil
}

// FindNewest takes the full path to a binary, and returns the newest
// update on disk. If there are no updates on disk, it returns the
// original path. It will return the same fullBinaryPath if that is
// the newest version.
func FindNewest(ctx context.Context, fullBinaryPath string, opts ...newestOption) string {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	if fullBinaryPath == "" {
		level.Debug(logger).Log("msg", "called with empty string")
		return ""
	}

	newestSettings := &newestSettings{}
	for _, opt := range opts {
		opt(newestSettings)
	}

	updateDir := getUpdateDir(fullBinaryPath)
	binaryName := filepath.Base(fullBinaryPath)

	logger = log.With(logger,
		"fullBinaryPath", fullBinaryPath,
		"updateDir", updateDir,
		"binaryName", binaryName,
	)

	// Find the possible updates. filepath.Glob returns a list of things
	// that match the requested pattern. We sort the list to ensure that
	// we can tell which ones are earlier or later (remember, these are
	// timestamps). If no updates are found, the forloop is skipped, and
	// we return eithier the seed fullBinaryPath or ""
	fileGlob := filepath.Join(updateDir, "*", binaryName)

	possibleUpdates, err := filepath.Glob(fileGlob)
	if err != nil {
		level.Error(logger).Log("msg", "globbing for updates", "err", err)
		return fullBinaryPath
	}

	sort.Strings(possibleUpdates)

	// iterate backwards over files, looking for a suitable binary
	foundCount := 0
	foundFile := ""
	for i := len(possibleUpdates) - 1; i >= 0; i-- {
		file := possibleUpdates[i]

		// If we've already found at least 2 files, (newest, and presumed
		// current), trigger delete routine
		if newestSettings.deleteOld && foundCount >= 2 {
			basedir := filepath.Dir(file)
			level.Debug(logger).Log("msg", "deleting old updates", "dir", basedir)
			if err := os.RemoveAll(basedir); err != nil {
				level.Error(logger).Log("msg", "error deleting old update dir", "dir", basedir, "err", err)
			}
		}

		// Sanity check that the executable is executable. Also remove the update if appropriate
		if err := checkExecutable(ctx, file, "--version"); err != nil {
			if newestSettings.deleteCorrupt {
				level.Error(logger).Log("msg", "not executable. Removing", "binary", file, "reason", err)
				basedir := filepath.Dir(file)
				if err := os.RemoveAll(basedir); err != nil {
					level.Error(logger).Log("msg", "error deleting broken update dir", "dir", basedir, "err", err)
				}
			} else {
				level.Error(logger).Log("msg", "not executable. Skipping", "binary", file, "reason", err)
			}

			continue
		}

		// We always want to increment the foundCount, since it's what triggers deletion.
		foundCount = foundCount + 1

		// Only set what we've found, if it's unset.
		if foundFile == "" {
			foundFile = file
		}
	}

	if foundFile != "" {
		return foundFile
	}

	level.Debug(logger).Log("msg", "no updates found")

	if newestSettings.skipFullBinaryPathCheck {
		return fullBinaryPath
	}

	if err := checkExecutable(ctx, fullBinaryPath, "--version"); err == nil {
		return fullBinaryPath
	}

	level.Debug(logger).Log("msg", "fullBinaryPath not executable. Returning nil")
	return ""
}

// getUpdateDir returns the expected update path for a given
// binary. It should work when called with either a base executable
// `/usr/local/bin/launcher` or with an existing updated
// `/usr/local/bin/launcher-updates/1234/launcher`.
//
// It makes some string assumptions about how things are named.
func getUpdateDir(fullBinaryPath string) string {
	// These are cases that shouldn't really happen. But, this is
	// a bare string function. So return "" when they do.
	if strings.HasSuffix(fullBinaryPath, "/") {
		fullBinaryPath = strings.TrimSuffix(fullBinaryPath, "/")
	}

	if fullBinaryPath == "" {
		return ""
	}

	// If we SplitN on updateDirSuffix, we will either get an
	// array, or the full string back. This means we can forgo a
	// strings.Contains, and just use the returned element
	components := strings.SplitN(fullBinaryPath, updateDirSuffix, 2)

	return fmt.Sprintf("%s%s", components[0], updateDirSuffix)
}

// FindBaseDir takes a binary path, that may or may not include the
// update directory, and returns the base directory. It's used by the
// launcher runtime in finding the various binaries.
func FindBaseDir(path string) string {
	if path == "" {
		return ""
	}

	components := strings.SplitN(path, updateDirSuffix, 2)
	return filepath.Dir(components[0])
}

// checkExecutable tests whether something is an executable. It
// examines permissions, mode, and tries to exec it directly.
func checkExecutable(ctx context.Context, potentialBinary string, args ...string) error {
	if err := checkExecutablePermissions(potentialBinary); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, potentialBinary, args...)
	execErr := cmd.Run()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return supressRoutineErrors(execErr)
}

// supressNormalErrors attempts to tell whether the error was a
// program that has executed, and then exited, vs one that's execution
// was entirely unsuccessful. This differentiation allows us to
// detect, and recover, from corrupt updates vs something in-app.
func supressRoutineErrors(err error) error {
	if err == nil {
		return nil
	}

	// Suppress exit codes of 1 or 2. These are generally indicative of
	// an unknown command line flag, _not_ a corrupt download. (exit
	// code 0 will be nil, and never trigger this block)
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() == 1 || exitError.ExitCode() == 2 {
			// suppress these
			return nil
		}
	}
	return err
}
