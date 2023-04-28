package tuf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	*readOnlyLibrary
	mirrorUrl    string // dl.kolide.co
	mirrorClient *http.Client
	stagingDir   string
	lock         *libraryLock
	logger       log.Logger
}

func newUpdateLibraryManager(mirrorUrl string, mirrorClient *http.Client, baseDir string, osquerier querier, logger log.Logger) (*updateLibraryManager, error) {
	// Ensure the updates directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("could not make base directory for updates library: %w", err)
	}

	// Create the directory for staging updates
	stagingDir, err := agent.MkdirTemp("staged-updates")
	if err != nil {
		return nil, fmt.Errorf("could not make staged updates directory: %w", err)
	}

	// Create the update library
	for _, binary := range binaries {
		if err := os.MkdirAll(filepath.Join(baseDir, string(binary)), 0755); err != nil {
			return nil, fmt.Errorf("could not make updates directory for %s: %w", binary, err)
		}
	}

	// Create the nested read-only library
	readOnlyLibrary, err := newReadOnlyLibrary(baseDir, osquerier, logger)
	if err != nil {
		return nil, fmt.Errorf("could not create read-only library: %w", err)
	}

	ulm := updateLibraryManager{
		readOnlyLibrary: readOnlyLibrary,
		mirrorUrl:       mirrorUrl,
		mirrorClient:    mirrorClient,
		stagingDir:      stagingDir,
		lock:            newLibraryLock(),
		logger:          log.With(logger, "component", "tuf_autoupdater_library_manager"),
	}

	return &ulm, nil
}

// AddToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there.
func (ulm *updateLibraryManager) AddToLibrary(binary autoupdatableBinary, targetFilename string, targetMetadata data.TargetFileMeta) error {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	if ulm.Available(binary, targetFilename) {
		return nil
	}

	// Remove downloaded archives after update, regardless of success -- this will run before the unlock
	defer ulm.tidyStagedUpdates(binary)

	stagedUpdatePath, err := ulm.stageAndVerifyUpdate(binary, targetFilename, targetMetadata)
	if err != nil {
		return fmt.Errorf("could not stage update: %w", err)
	}

	if err := ulm.moveVerifiedUpdate(binary, targetFilename, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	return nil
}

// stageAndVerifyUpdate downloads the update indicated by `targetFilename` and verifies it against
// the given, validated local metadata.
func (ulm *updateLibraryManager) stageAndVerifyUpdate(binary autoupdatableBinary, targetFilename string, localTargetMetadata data.TargetFileMeta) (string, error) {
	stagedUpdatePath := filepath.Join(ulm.stagingDir, targetFilename)

	// Request download from mirror
	resp, err := ulm.mirrorClient.Get(ulm.mirrorUrl + fmt.Sprintf("/kolide/%s/%s/%s", binary, runtime.GOOS, targetFilename))
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not make request to download target %s: %w", targetFilename, err)
	}
	defer resp.Body.Close()

	// Wrap the download in a LimitReader so we read at most localMeta.Length bytes
	stream := io.LimitReader(resp.Body, localTargetMetadata.Length)
	var fileBuffer bytes.Buffer

	// Read the target file, simultaneously writing it to our file buffer and generating its metadata
	actualTargetMeta, err := tufutil.GenerateTargetFileMeta(io.TeeReader(stream, io.Writer(&fileBuffer)), localTargetMetadata.HashAlgorithms()...)
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s and compute its metadata: %w", targetFilename, stagedUpdatePath, err)
	}

	// Verify the actual download against the confirmed local metadata
	if err := tufutil.TargetFileMetaEqual(actualTargetMeta, localTargetMetadata); err != nil {
		return stagedUpdatePath, fmt.Errorf("verification failed for target %s staged at %s: %w", targetFilename, stagedUpdatePath, err)
	}

	// Everything looks good: create the file and write it to disk
	out, err := os.Create(stagedUpdatePath)
	if err != nil {
		return "", fmt.Errorf("could not create file at %s: %w", stagedUpdatePath, err)
	}
	if _, err := io.Copy(out, &fileBuffer); err != nil {
		out.Close()
		return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s: %w", targetFilename, stagedUpdatePath, err)
	}
	if err := out.Close(); err != nil {
		return stagedUpdatePath, fmt.Errorf("could not close downloaded target file %s after writing: %w", targetFilename, err)
	}

	return stagedUpdatePath, nil
}

// moveVerifiedUpdate untars the update and performs final checks to make sure that it's a valid, working update.
// Finally, it moves the update to its correct versioned location in the update library for the given binary.
func (ulm *updateLibraryManager) moveVerifiedUpdate(binary autoupdatableBinary, targetFilename string, stagedUpdate string) error {
	targetVersion := ulm.versionFromTarget(binary, targetFilename)
	stagedVersionedDirectory := filepath.Join(ulm.stagingDir, targetVersion)
	if err := os.MkdirAll(stagedVersionedDirectory, 0755); err != nil {
		return fmt.Errorf("could not create directory %s for untarring and validating new update: %w", stagedVersionedDirectory, err)
	}

	// Untar the archive. Note that `UntarBundle` calls `filepath.Dir(destination)`, so the inclusion of `binary`
	// here doesn't matter as it's immediately stripped off.
	if err := fsutil.UntarBundle(filepath.Join(stagedVersionedDirectory, string(binary)), stagedUpdate); err != nil {
		ulm.removeUpdate(binary, targetVersion)
		return fmt.Errorf("could not untar update to %s: %w", stagedVersionedDirectory, err)
	}

	// Make sure that the binary is executable
	if err := os.Chmod(executableLocation(stagedVersionedDirectory, binary), 0755); err != nil {
		ulm.removeUpdate(binary, targetVersion)
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// Validate the executable
	if err := autoupdate.CheckExecutable(context.TODO(), executableLocation(stagedVersionedDirectory, binary), "--version"); err != nil {
		ulm.removeUpdate(binary, targetVersion)
		return fmt.Errorf("could not verify executable: %w", err)
	}

	// All good! Shelve it in the library under its version
	newUpdateDirectory := filepath.Join(ulm.updatesDirectory(binary), targetVersion)
	if err := os.Rename(stagedVersionedDirectory, newUpdateDirectory); err != nil {
		return fmt.Errorf("could not move staged target %s from %s to %s: %w", targetFilename, stagedVersionedDirectory, newUpdateDirectory, err)
	}

	return nil
}

// removeUpdate removes a given version from the given binary's update library.
func (ulm *updateLibraryManager) removeUpdate(binary autoupdatableBinary, binaryVersion string) {
	directoryToRemove := filepath.Join(ulm.updatesDirectory(binary), binaryVersion)
	if err := os.RemoveAll(directoryToRemove); err != nil {
		level.Debug(ulm.logger).Log("msg", "could not remove update", "err", err, "directory", directoryToRemove)
	} else {
		level.Debug(ulm.logger).Log("msg", "removed update", "directory", directoryToRemove)
	}
}

// TidyLibrary removes unneeded files from the staged updates and updates directories.
func (ulm *updateLibraryManager) TidyLibrary() {
	for _, binary := range binaries {
		// Acquire lock for modifying the library
		ulm.lock.Lock(binary)
		defer ulm.lock.Unlock(binary)

		// First, remove old staged archives
		ulm.tidyStagedUpdates(binary)

		// Get the current running version to preserve it when tidying the available updates
		var currentVersion string
		var err error
		switch binary {
		case binaryOsqueryd:
			// The osqueryd client may not have initialized yet, so retry the version
			// check a couple times before giving up
			osquerydVersionCheckRetries := 5
			for i := 0; i < osquerydVersionCheckRetries; i += 1 {
				currentVersion, err = ulm.currentRunningVersion(binary)
				if err == nil {
					break
				}
				time.Sleep(1 * time.Minute)
			}
		default:
			currentVersion, err = ulm.currentRunningVersion(binary)
		}

		if err != nil {
			level.Debug(ulm.logger).Log("msg", "could not get current running version", "binary", binary, "err", err)
			continue
		}

		ulm.tidyUpdateLibrary(binary, currentVersion)
	}
}

// tidyStagedUpdates removes all old archives from the staged updates directory.
func (ulm *updateLibraryManager) tidyStagedUpdates(binary autoupdatableBinary) {
	matches, err := filepath.Glob(filepath.Join(ulm.stagingDir, "*"))
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not glob for staged updates to tidy updates library", "err", err)
		return
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove staged update when tidying update library", "file", match, "err", err)
		}
	}
}

// tidyUpdateLibrary reviews all updates in the library for the binary and removes any old versions
// that are no longer needed. It will always preserve the current running binary, and then the
// two most recent valid versions. It will remove versions it cannot validate.
func (ulm *updateLibraryManager) tidyUpdateLibrary(binary autoupdatableBinary, currentRunningVersion string) {
	if currentRunningVersion == "" {
		level.Debug(ulm.logger).Log("msg", "cannot tidy update library without knowing current running version")
		return
	}

	const numberOfVersionsToKeep = 3

	versionsInLibrary, invalidVersionsInLibrary, err := ulm.sortedVersionsInLibrary(binary)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not get versions in library to tidy update library", "err", err)
		return
	}

	for _, invalidVersion := range invalidVersionsInLibrary {
		level.Debug(ulm.logger).Log("msg", "updates library contains invalid version", "err", err, "library_path", invalidVersion)
		ulm.removeUpdate(binary, invalidVersion)
	}

	if len(versionsInLibrary) <= numberOfVersionsToKeep {
		return
	}

	// Loop through, looking at the most recent versions first, and remove all once we hit nonCurrentlyRunningVersionsKept valid executables
	nonCurrentlyRunningVersionsKept := 0
	for i := len(versionsInLibrary) - 1; i >= 0; i -= 1 {
		// Always keep the current running executable
		if versionsInLibrary[i] == currentRunningVersion {
			continue
		}

		// If we've already hit the number of versions to keep, then start to remove the older ones.
		// We want to keep numberOfVersionsToKeep total, saving a spot for the currently running version.
		if nonCurrentlyRunningVersionsKept >= numberOfVersionsToKeep-1 {
			ulm.removeUpdate(binary, versionsInLibrary[i])
			continue
		}

		nonCurrentlyRunningVersionsKept += 1
	}
}
