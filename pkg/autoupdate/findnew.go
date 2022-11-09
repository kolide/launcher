package autoupdate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// defaultBuildTimestamp is used to set the _oldest_ allowed update. Eg, if
// there's an update with a download timestamp older than build, just
// ignore it. It's probably indicative of a machine that's been re-installed.
//
// This is a private variable. It should be set via build time
// LDFLAGS.
const defaultBuildTimestamp = "0"

// This suffix is added to the binary path to find the updates
const updateDirSuffix = "-updates"

type newestSettings struct {
	deleteOld               bool
	deleteCorrupt           bool
	skipFullBinaryPathCheck bool
	buildTimestamp          string
	runningExecutable       string
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

// overrideBuildTimestamp overrides the buildTimestamp constant. This
// is to allow tests to mock that behavior. It is not exported, as it
// is expected to only be used for testing.
func overrideBuildTimestamp(ts string) newestOption {
	return func(no *newestSettings) {
		no.buildTimestamp = ts
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

// withRunningExectuable sets the current executable. This is because
// we never need to run an executable check against ourselves. (And
// doing so will trigger a fork bomb)
func withRunningExectuable(exe string) newestOption {
	return func(no *newestSettings) {
		no.runningExecutable = exe
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

	opts = append(opts, SkipFullBinaryPathCheck(), withRunningExectuable(exPath))

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

	newestSettings := &newestSettings{
		buildTimestamp: defaultBuildTimestamp,
	}
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

	// If no updates are found, the forloop is skipped, and we return either the seed fullBinaryPath or ""
	possibleUpdates, err := getPossibleUpdates(updateDir, binaryName)
	if err != nil {
		level.Error(logger).Log("msg", "could not find possible updates", "err", err)
		return ""
	}

	// iterate backwards over files, looking for a suitable binary
	foundCount := 0
	foundFile := ""
	for i := len(possibleUpdates) - 1; i >= 0; i-- {
		file := possibleUpdates[i]
		basedir := filepath.Dir(file)
		updateDownloadTime := filepath.Base(basedir)
		foundExecutable := file
		if strings.HasSuffix(file, ".app") {
			foundExecutable = filepath.Join(file, "Contents", "MacOS", binaryName)
		}

		// We only want to consider updates with a download time _newer_
		// than our build timestamp. Note that we're not comparing against
		// the update's build time, only the download time. This is an
		// important distinction to allow for downgrades.
		if strings.Compare(newestSettings.buildTimestamp, updateDownloadTime) >= 0 {
			level.Debug(logger).Log(
				"msg", "update download is older than buildtime",
				"dir", basedir,
				"buildtime", newestSettings.buildTimestamp,
			)

			if newestSettings.deleteOld {
				if err := os.RemoveAll(basedir); err != nil {
					level.Error(logger).Log("msg", "error deleting old update dir", "dir", basedir, "err", err)
				}
			}

			continue
		}

		// If we've already found at least 2 files, (newest, and presumed
		// current), trigger delete routine
		if newestSettings.deleteOld && foundCount >= 2 {
			level.Debug(logger).Log("msg", "deleting old updates", "dir", basedir)
			if err := os.RemoveAll(basedir); err != nil {
				level.Error(logger).Log("msg", "error deleting old update dir", "dir", basedir, "err", err)
			}
		}

		// If the file is _not_ the running executable, sanity
		// check that executions work. If the exec fails,
		// there's clearly an issue and we should remove it.
		if newestSettings.runningExecutable != foundExecutable {
			if err := checkExecutable(ctx, foundExecutable, "--version"); err != nil {
				if newestSettings.deleteCorrupt {
					level.Error(logger).Log("msg", "not executable. Removing", "binary", foundExecutable, "reason", err)
					if err := os.RemoveAll(basedir); err != nil {
						level.Error(logger).Log("msg", "error deleting broken update dir", "dir", basedir, "err", err)
					}
				} else {
					level.Error(logger).Log("msg", "not executable. Skipping", "binary", foundExecutable, "reason", err)
				}

				continue
			}
		} else {
			// This logging is mostly here to make test coverage of the conditional clear
			level.Debug(logger).Log("msg", "Skipping checkExecutable against self", "file", file)
		}

		// We always want to increment the foundCount, since it's what triggers deletion.
		foundCount = foundCount + 1

		// Only set what we've found, if it's unset.
		if foundFile == "" {
			foundFile = foundExecutable
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

// Find the possible updates. filepath.Glob returns a list of things
// that match the requested pattern. We sort the list to ensure that
// we can tell which ones are earlier or later (remember, these are
// timestamps).
func getPossibleUpdates(updateDir, binaryName string) ([]string, error) {
	// If this is launcher running on macOS, then we should have app bundles available instead --
	// check for those first.
	if runtime.GOOS == "darwin" {
		binarySuffix := filepath.Join("Contents", "MacOS", binaryName)
		fileGlob := filepath.Join(updateDir, "*", "*.app", binarySuffix)
		possibleUpdates, err := filepath.Glob(fileGlob)

		if err == nil && len(possibleUpdates) > 0 {
			appBundleNames := make([]string, len(possibleUpdates))
			for i, binaryPath := range possibleUpdates {
				appBundleNames[i] = strings.TrimSuffix(binaryPath, "/"+binarySuffix)
			}
			sort.Strings(appBundleNames)
			return appBundleNames, nil
		}
	}

	// Either not macOS/launcher or no app bundles found. Fall back to searching for binaries.
	fileGlob := filepath.Join(updateDir, "*", binaryName)

	possibleUpdates, err := filepath.Glob(fileGlob)
	if err != nil {
		return nil, err
	}

	sort.Strings(possibleUpdates)

	return possibleUpdates, nil
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

	// If we can determine that the requested executable is
	// ourself, don't try to exec. It's needless, and a potential
	// fork bomb. Ignore errors, either we get an answer or we don't.
	selfPath, _ := os.Executable()
	if filepath.Clean(selfPath) == filepath.Clean(potentialBinary) {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, potentialBinary, args...)

	// Set env, this should prevent launcher for fork-bombing
	cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE")

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
