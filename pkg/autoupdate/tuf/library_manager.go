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
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/localserver"
	"github.com/kolide/launcher/pkg/autoupdate"
	client "github.com/theupdateframework/go-tuf/client"
)

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	metadataClient  *client.Client // used to validate downloads
	mirrorUrl       string         // dl.kolide.co
	mirrorClient    *http.Client
	rootDirectory   string
	operatingSystem string
	osquerier       localserver.Querier // used to query for current running osquery version
	logger          log.Logger
}

func newUpdateLibraryManager(metadataClient *client.Client, mirrorUrl string, mirrorClient *http.Client, rootDirectory string, operatingSystem string, osquerier localserver.Querier, logger log.Logger) (*updateLibraryManager, error) {
	ulm := updateLibraryManager{
		metadataClient:  metadataClient,
		mirrorUrl:       mirrorUrl,
		mirrorClient:    mirrorClient,
		rootDirectory:   rootDirectory,
		operatingSystem: operatingSystem,
		osquerier:       osquerier,
		logger:          logger,
	}

	// Ensure our staged updates and updates directories exist
	for _, binary := range binaries {
		if err := os.MkdirAll(ulm.stagedUpdatesDirectory(binary), 0755); err != nil {
			return nil, fmt.Errorf("could not make staged updates directory for %s: %w", binary, err)
		}

		if err := os.MkdirAll(ulm.updatesDirectory(binary), 0755); err != nil {
			return nil, fmt.Errorf("could not make staged updates directory for %s: %w", binary, err)
		}
	}

	return &ulm, nil
}

// updatesDirectory returns the update library location for the given binary.
func (ulm *updateLibraryManager) updatesDirectory(binary string) string {
	return filepath.Join(ulm.rootDirectory, fmt.Sprintf("%s-updates", binary))
}

// stagedUpdatesDirectory returns the location for staged updates -- i.e. updates
// that have been downloaded, but not yet verified.
func (ulm *updateLibraryManager) stagedUpdatesDirectory(binary string) string {
	return filepath.Join(ulm.rootDirectory, fmt.Sprintf("%s-staged-updates", binary))
}

// addToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there. After any addition
// to the library, it cleans up older versions that are no longer needed.
func (ulm *updateLibraryManager) AddToLibrary(binary string, targetFilename string) error {
	// Check to see if the current running version is the version we were requested to add;
	// return early if it is, but don't error out if we can't determine the current version.
	currentVersion, err := ulm.currentRunningVersion(binary)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not get current running version", "binary", binary, "err", err)
	} else {
		if currentVersion.Original() == ulm.versionFromTarget(binary, targetFilename) {
			// We don't need to download the current running version because it already exists,
			// either in this updates library or in the original install location.
			return nil
		}
	}

	if ulm.alreadyAdded(binary, targetFilename) {
		return nil
	}

	stagedUpdatePath, err := ulm.stageUpdate(binary, targetFilename)
	defer func() {
		if err := os.Remove(stagedUpdatePath); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove staged update", "staged_update_path", stagedUpdatePath, "err", err)
		}
	}()
	if err != nil {
		return fmt.Errorf("could not stage update: %w", err)
	}

	if err := ulm.verifyStagedUpdate(binary, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not verify staged update: %w", err)
	}

	if err := ulm.moveVerifiedUpdate(binary, targetFilename, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	if currentVersion != nil {
		ulm.tidyLibrary(binary, currentVersion)
	} else {
		level.Debug(ulm.logger).Log("msg", "skipping tidying library because current running version could not be determined", "binary", binary)
	}

	return nil
}

// alreadyAdded checks if the given target already exists in the update library.
func (ulm *updateLibraryManager) alreadyAdded(binary string, targetFilename string) bool {
	updateDirectory := filepath.Join(ulm.updatesDirectory(binary), ulm.versionFromTarget(binary, targetFilename))

	return autoupdate.CheckExecutable(context.TODO(), executableLocation(updateDirectory, binary), "--version") == nil
}

// versionFromTarget extracts the semantic version for an update from its filename.
func (ulm *updateLibraryManager) versionFromTarget(binary string, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

// stageUpdate downloads the update indicated by `targetFilename` and stages it for
// further verification.
func (ulm *updateLibraryManager) stageUpdate(binary string, targetFilename string) (string, error) {
	stagedUpdatePath := filepath.Join(ulm.stagedUpdatesDirectory(binary), targetFilename)
	out, err := os.Create(stagedUpdatePath)
	if err != nil {
		return "", fmt.Errorf("could not create file at %s: %w", stagedUpdatePath, err)
	}
	defer out.Close()

	resp, err := ulm.mirrorClient.Get(ulm.mirrorUrl + fmt.Sprintf("/kolide/%s/%s/%s", binary, ulm.operatingSystem, targetFilename))
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
func (ulm *updateLibraryManager) verifyStagedUpdate(binary string, stagedUpdate string) error {
	digest, err := sha512Digest(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not compute digest for target %s to verify it: %w", stagedUpdate, err)
	}

	fileInfo, err := os.Stat(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not get info for staged update at %s: %w", stagedUpdate, err)
	}

	// Where the file lives in the binary bucket
	pathToTargetInMirror := filepath.Join(binary, ulm.operatingSystem, filepath.Base(stagedUpdate))
	if err := ulm.metadataClient.VerifyDigest(digest, "sha512", fileInfo.Size(), pathToTargetInMirror); err != nil {
		return fmt.Errorf("digest verification failed for target staged at %s: %w", stagedUpdate, err)
	}

	return nil
}

// moveVerifiedUpdate untars the update, moves it into the update library, and performs final checks
// to make sure that it's a valid, working update.
func (ulm *updateLibraryManager) moveVerifiedUpdate(binary string, targetFilename string, stagedUpdate string) error {
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

	if err := fsutil.UntarBundle(filepath.Join(newUpdateDirectory, binary), stagedUpdate); err != nil {
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
func (ulm *updateLibraryManager) currentRunningVersion(binary string) (*semver.Version, error) {
	switch binary {
	case binaryLauncher:
		currentVersion, err := semver.NewVersion(version.Version().Version)
		if err != nil {
			return nil, fmt.Errorf("cannot determine current running version of launcher: %w", err)
		}
		return currentVersion, nil
	case binaryOsqueryd:
		resp, err := ulm.osquerier.Query("SELECT version FROM osquery_info;")
		if err != nil {
			return nil, fmt.Errorf("could not query for osquery version: %w", err)
		}
		if len(resp) < 1 {
			return nil, errors.New("osquery version query returned no rows")
		}
		rawVersion, ok := resp[0]["version"]
		if !ok {
			return nil, errors.New("osquery version query did not return version")
		}

		currentVersion, err := semver.NewVersion(rawVersion)
		if err != nil {
			return nil, fmt.Errorf("could not parse current running version %s of osquery as semver: %w", rawVersion, err)
		}

		return currentVersion, nil
	default:
		return nil, fmt.Errorf("cannot determine current running version for unexpected binary %s", binary)
	}
}

// tidyLibrary reviews all updates in the library for the binary and removes any old versions
// that are no longer needed. It will always preserve the current running binary, and then the
// two most recent valid versions. It will remove versions it cannot validate.
func (ulm *updateLibraryManager) tidyLibrary(binary string, currentRunningVersion *semver.Version) {
	const numberOfVersionsToKeep = 3

	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(ulm.updatesDirectory(binary), "*"))
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not glob for updates to tidy updates library", "err", err)
		return
	}

	if len(rawVersionsInLibrary) <= numberOfVersionsToKeep {
		return
	}

	removeUpdate := func(v string) {
		directoryToRemove := filepath.Join(ulm.updatesDirectory(binary), v)
		level.Debug(ulm.logger).Log("removing old update", "directory", directoryToRemove)
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

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Loop through, looking at the most recent versions first, and remove all once we hit nonCurrentlyRunningVersionsKept valid executables
	nonCurrentlyRunningVersionsKept := 0
	for i := len(versionsInLibrary) - 1; i >= 0; i -= 1 {
		// Always keep the current running executable
		if versionsInLibrary[i].Equal(currentRunningVersion) {
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
