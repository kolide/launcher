package tuf

import (
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	client "github.com/theupdateframework/go-tuf/client"
)

type querier interface {
	Query(query string) ([]map[string]string, error)
}

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	metadataClient *client.Client // used to validate downloads
	mirrorUrl      string         // dl.kolide.co
	mirrorClient   *http.Client
	baseDir        string
	osquerier      querier // used to query for current running osquery version
	logger         log.Logger
}

func newUpdateLibraryManager(metadataClient *client.Client, mirrorUrl string, mirrorClient *http.Client, baseDir string, osquerier querier, logger log.Logger) (*updateLibraryManager, error) {
	ulm := updateLibraryManager{
		metadataClient: metadataClient,
		mirrorUrl:      mirrorUrl,
		mirrorClient:   mirrorClient,
		baseDir:        baseDir,
		osquerier:      osquerier,
		logger:         log.With(logger, "component", "tuf_autoupdater_library_manager"),
	}

	// Ensure the updates directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("could not make base directory for updates library: %w", err)
	}

	// Ensure our staged updates and updates directories exist
	for _, binary := range binaries {
		// Create the directory for staging updates
		if err := os.MkdirAll(ulm.stagedUpdatesDirectory(binary), 0755); err != nil {
			return nil, fmt.Errorf("could not make staged updates directory for %s: %w", binary, err)
		}

		// Create the update library
		if err := os.MkdirAll(ulm.updatesDirectory(binary), 0755); err != nil {
			return nil, fmt.Errorf("could not make updates directory for %s: %w", binary, err)
		}
	}

	return &ulm, nil
}

// updatesDirectory returns the update library location for the given binary.
func (ulm *updateLibraryManager) updatesDirectory(binary autoupdatableBinary) string {
	return filepath.Join(ulm.baseDir, string(binary))
}

// stagedUpdatesDirectory returns the location for staged updates -- i.e. updates
// that have been downloaded, but not yet verified.
func (ulm *updateLibraryManager) stagedUpdatesDirectory(binary autoupdatableBinary) string {
	return filepath.Join(ulm.baseDir, fmt.Sprintf("%s-staged", binary))
}

// addToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there.
func (ulm *updateLibraryManager) AddToLibrary(binary autoupdatableBinary, targetFilename string) error {
	// Check to see if the current running version is the version we were requested to add;
	// return early if it is, but don't error out if we can't determine the current version.
	currentVersion, err := ulm.currentRunningVersion(binary)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not get current running version", "binary", binary, "err", err)
	} else if currentVersion == ulm.versionFromTarget(binary, targetFilename) {
		// We don't need to download the current running version because it already exists,
		// either in this updates library or in the original install location.
		return nil
	}

	if ulm.alreadyAdded(binary, targetFilename) {
		return nil
	}

	// Remove downloaded archives after update, regardless of success
	defer ulm.tidyStagedUpdates(binary)

	stagedUpdatePath, err := ulm.stageUpdate(binary, targetFilename)
	if err != nil {
		return fmt.Errorf("could not stage update: %w", err)
	}

	if err := ulm.verifyStagedUpdate(binary, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not verify staged update: %w", err)
	}

	if err := ulm.moveVerifiedUpdate(binary, targetFilename, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	return nil
}

// alreadyAdded checks if the given target already exists in the update library.
func (ulm *updateLibraryManager) alreadyAdded(binary autoupdatableBinary, targetFilename string) bool {
	updateDirectory := filepath.Join(ulm.updatesDirectory(binary), ulm.versionFromTarget(binary, targetFilename))

	return autoupdate.CheckExecutable(context.TODO(), executableLocation(updateDirectory, binary), "--version") == nil
}

// versionFromTarget extracts the semantic version for an update from its filename.
func (ulm *updateLibraryManager) versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

// stageUpdate downloads the update indicated by `targetFilename` and stages it for
// further verification.
func (ulm *updateLibraryManager) stageUpdate(binary autoupdatableBinary, targetFilename string) (string, error) {
	stagedUpdatePath := filepath.Join(ulm.stagedUpdatesDirectory(binary), targetFilename)
	out, err := os.Create(stagedUpdatePath)
	if err != nil {
		return "", fmt.Errorf("could not create file at %s: %w", stagedUpdatePath, err)
	}
	defer out.Close()

	resp, err := ulm.mirrorClient.Get(ulm.mirrorUrl + fmt.Sprintf("/kolide/%s/%s/%s", binary, runtime.GOOS, targetFilename))
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not make request to download target %s: %w", targetFilename, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s: %w", targetFilename, stagedUpdatePath, err)
	}

	return stagedUpdatePath, nil
}

// verifyStagedUpdate checks the downloaded update against the metadata in the TUF repo.
func (ulm *updateLibraryManager) verifyStagedUpdate(binary autoupdatableBinary, stagedUpdate string) error {
	digest, err := sha512Digest(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not compute digest for target %s to verify it: %w", stagedUpdate, err)
	}

	fileInfo, err := os.Stat(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not get info for staged update at %s: %w", stagedUpdate, err)
	}

	// Where the file lives in the binary bucket -- we can't use filepath.Join here because on Windows,
	// that won't match the actual bucket filepath
	pathToTargetInMirror := fmt.Sprintf("%s/%s/%s", binary, runtime.GOOS, filepath.Base(stagedUpdate))
	if err := ulm.metadataClient.VerifyDigest(digest, "sha512", fileInfo.Size(), pathToTargetInMirror); err != nil {
		return fmt.Errorf("digest verification failed for target staged at %s: %w", stagedUpdate, err)
	}

	return nil
}

// moveVerifiedUpdate untars the update, moves it into the update library, and performs final checks
// to make sure that it's a valid, working update.
func (ulm *updateLibraryManager) moveVerifiedUpdate(binary autoupdatableBinary, targetFilename string, stagedUpdate string) error {
	newUpdateDirectory := filepath.Join(ulm.updatesDirectory(binary), ulm.versionFromTarget(binary, targetFilename))
	if err := os.MkdirAll(newUpdateDirectory, 0755); err != nil {
		return fmt.Errorf("could not create directory %s for new update: %w", newUpdateDirectory, err)
	}

	removeBrokenUpdateDir := func() {
		if err := os.RemoveAll(newUpdateDirectory); err != nil {
			level.Debug(ulm.logger).Log(
				"msg", "could not remove broken update directory",
				"update_dir", newUpdateDirectory,
				"err", err,
			)
		}
	}

	if err := fsutil.UntarBundle(filepath.Join(newUpdateDirectory, string(binary)), stagedUpdate); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not untar update to %s: %w", newUpdateDirectory, err)
	}

	// Make sure that the binary is executable
	if err := os.Chmod(executableLocation(newUpdateDirectory, binary), 0755); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// Validate the executable
	if err := autoupdate.CheckExecutable(context.TODO(), executableLocation(newUpdateDirectory, binary), "--version"); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not verify executable: %w", err)
	}

	return nil
}

// currentRunningVersion returns the current running version of the given binary.
func (ulm *updateLibraryManager) currentRunningVersion(binary autoupdatableBinary) (string, error) {
	switch binary {
	case binaryLauncher:
		launcherVersion := version.Version().Version
		if launcherVersion == "unknown" {
			return "", errors.New("unknown launcher version")
		}
		return launcherVersion, nil
	case binaryOsqueryd:
		resp, err := ulm.osquerier.Query("SELECT version FROM osquery_info;")
		if err != nil {
			return "", fmt.Errorf("could not query for osquery version: %w", err)
		}
		if len(resp) < 1 {
			return "", errors.New("osquery version query returned no rows")
		}
		osquerydVersion, ok := resp[0]["version"]
		if !ok {
			return "", errors.New("osquery version query did not return version")
		}

		return osquerydVersion, nil
	default:
		return "", fmt.Errorf("cannot determine current running version for unexpected binary %s", binary)
	}
}

// TidyLibrary removes unneeded files from the staged updates and updates directories.
func (ulm *updateLibraryManager) TidyLibrary() {
	for _, binary := range binaries {
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
	matches, err := filepath.Glob(filepath.Join(ulm.stagedUpdatesDirectory(binary), "*"))
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

	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(ulm.updatesDirectory(binary), "*"))
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not glob for updates to tidy updates library", "err", err)
		return
	}

	removeUpdate := func(v string) {
		directoryToRemove := filepath.Join(ulm.updatesDirectory(binary), v)
		level.Debug(ulm.logger).Log("msg", "removing old update", "directory", directoryToRemove)
		if err := os.RemoveAll(directoryToRemove); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove old update when tidying updates library", "err", err, "directory", directoryToRemove)
		}
	}

	versionsInLibrary := make([]*semver.Version, 0)
	for _, rawVersion := range rawVersionsInLibrary {
		v, err := semver.NewVersion(filepath.Base(rawVersion))
		if err != nil {
			level.Debug(ulm.logger).Log("msg", "updates library contains invalid semver", "err", err, "library_path", rawVersion)
			removeUpdate(filepath.Base(rawVersion))
			continue
		}

		versionsInLibrary = append(versionsInLibrary, v)
	}

	if len(versionsInLibrary) <= numberOfVersionsToKeep {
		return
	}

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Loop through, looking at the most recent versions first, and remove all once we hit nonCurrentlyRunningVersionsKept valid executables
	nonCurrentlyRunningVersionsKept := 0
	for i := len(versionsInLibrary) - 1; i >= 0; i -= 1 {
		// Always keep the current running executable
		if versionsInLibrary[i].Original() == currentRunningVersion {
			continue
		}

		// If we've already hit the number of versions to keep, then start to remove the older ones.
		// We want to keep numberOfVersionsToKeep total, saving a spot for the currently running version.
		if nonCurrentlyRunningVersionsKept >= numberOfVersionsToKeep-1 {
			removeUpdate(versionsInLibrary[i].Original())
			continue
		}

		// Only keep good executables
		versionDir := filepath.Join(ulm.updatesDirectory(binary), versionsInLibrary[i].Original())
		if err := autoupdate.CheckExecutable(context.TODO(), executableLocation(versionDir, binary), "--version"); err != nil {
			removeUpdate(versionsInLibrary[i].Original())
			continue
		}

		nonCurrentlyRunningVersionsKept += 1
	}
}

// sha512Digest calculates the digest of the given file, for use in validating downloads from the mirror
// against the local TUF repository.
func sha512Digest(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("could not open file %s to calculate digest: %w", filename, err)
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("could not compute checksum for file %s: %w", filename, err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
