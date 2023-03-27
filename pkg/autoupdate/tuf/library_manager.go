package tuf

import (
	"crypto/sha512"
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
	client "github.com/theupdateframework/go-tuf/client"
)

type updateLibraryManager struct {
	metadataClient  *client.Client
	mirrorUrl       string
	mirrorClient    *http.Client
	rootDirectory   string
	operatingSystem string
	logger          log.Logger
}

func newUpdateLibraryManager(metadataClient *client.Client, mirrorUrl string, mirrorClient *http.Client, rootDirectory string, operatingSystem string, logger log.Logger) (*updateLibraryManager, error) {
	ulm := updateLibraryManager{
		metadataClient:  metadataClient,
		mirrorUrl:       mirrorUrl,
		mirrorClient:    mirrorClient,
		rootDirectory:   rootDirectory,
		operatingSystem: operatingSystem,
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

func (ulm *updateLibraryManager) updatesDirectory(binary string) string {
	return filepath.Join(ulm.rootDirectory, fmt.Sprintf("%s-updates", binary))
}

func (ulm *updateLibraryManager) stagedUpdatesDirectory(binary string) string {
	return filepath.Join(ulm.rootDirectory, fmt.Sprintf("%s-staged-updates", binary))
}

func (ulm *updateLibraryManager) addToLibrary(binary string, targetFilename string) error {
	if ulm.alreadyAdded(binary, targetFilename) {
		return nil
	}

	if err := ulm.stageUpdate(binary, targetFilename); err != nil {
		return fmt.Errorf("could not stage update: %w", err)
	}

	if err := ulm.verifyStagedUpdate(binary, targetFilename); err != nil {
		return fmt.Errorf("could not verify staged update: %w", err)
	}

	if err := ulm.moveVerifiedUpdate(binary, targetFilename); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	v, err := ulm.currentRunningVersion(binary)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "cannot parse current running version, cannot safely tidy updates library", "err", err)
		return nil
	}

	ulm.tidyLibrary(binary, v)

	return nil
}

func (ulm *updateLibraryManager) alreadyAdded(binary string, targetFilename string) bool {
	updateDirectory := filepath.Join(ulm.updatesDirectory(binary), ulm.versionFromTarget(binary, targetFilename))

	return ulm.verifyExecutableInDirectory(updateDirectory, binary) == nil
}

func (ulm *updateLibraryManager) versionFromTarget(binary string, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

func (ulm *updateLibraryManager) stageUpdate(binary string, targetFilename string) error {
	stagedUpdatePath := filepath.Join(ulm.stagedUpdatesDirectory(binary), targetFilename)
	out, err := os.Create(stagedUpdatePath)
	if err != nil {
		return fmt.Errorf("could not create file at %s: %w", stagedUpdatePath, err)
	}
	defer out.Close()

	resp, err := ulm.mirrorClient.Get(ulm.mirrorUrl + fmt.Sprintf("/kolide/%s", targetFilename))
	if err != nil {
		return fmt.Errorf("could not make request to download target %s: %w", targetFilename, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("could not write downloaded target %s to file %s: %w", targetFilename, stagedUpdatePath, err)
	}

	return nil
}

func (ulm *updateLibraryManager) verifyStagedUpdate(binary string, targetFilename string) error {
	stagedUpdate := filepath.Join(ulm.stagedUpdatesDirectory(binary), targetFilename)
	digest, err := sha512Digest(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not compute digest for target %s to verify it: %w", targetFilename, err)
	}

	fileInfo, err := os.Stat(stagedUpdate)
	if err != nil {
		return fmt.Errorf("could not get info for staged update at %s: %w", stagedUpdate, err)
	}

	// Where the file lives in the binary bucket
	pathToTargetInMirror := filepath.Join(binary, ulm.operatingSystem, targetFilename)
	if err := ulm.metadataClient.VerifyDigest(digest, "sha512", fileInfo.Size(), pathToTargetInMirror); err != nil {
		return fmt.Errorf("digest verification failed for target %s staged at %s: %w", targetFilename, stagedUpdate, err)
	}

	return nil
}

func (ulm *updateLibraryManager) moveVerifiedUpdate(binary string, targetFilename string) error {
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

	if err := fsutil.UntarBundle(filepath.Join(newUpdateDirectory, binary), filepath.Join(ulm.stagedUpdatesDirectory(binary), targetFilename)); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not untar update to %s: %w", newUpdateDirectory, err)
	}

	// Make sure that the binary is executable
	if err := os.Chmod(executableLocation(newUpdateDirectory, binary), 0755); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// Validate the executable
	if err := ulm.verifyExecutableInDirectory(newUpdateDirectory, binary); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not verify executable: %w", err)
	}

	return nil
}

func (ulm *updateLibraryManager) verifyExecutableInDirectory(updateDirectory string, binary string) error {
	stat, err := os.Stat(executableLocation(updateDirectory, binary))
	switch {
	case os.IsNotExist(err):
		// Target has not been downloaded
		return fmt.Errorf("file does not exist at %s", updateDirectory)
	case stat.IsDir():
		return fmt.Errorf("expected executable but got directory at %s", updateDirectory)
	case err != nil:
		// Can't check -- assume it's not added
		return fmt.Errorf("error checking file info for %s: %w", updateDirectory, err)
	case stat.Mode()&0111 == 0:
		// Exists but not executable
		return fmt.Errorf("file %s is not executable", updateDirectory)
	}

	// TODO run with --version

	return nil
}

func (ulm *updateLibraryManager) currentRunningVersion(binary string) (*semver.Version, error) {
	switch binary {
	case "launcher":
		currentVersion, err := semver.NewVersion(version.Version().Version)
		if err != nil {
			return nil, fmt.Errorf("cannot determine current running version of launcher: %w", err)
		}
		return currentVersion, nil
	case "osqueryd":
		// TODO
		return nil, fmt.Errorf("not implemented for %s yet", binary)
	default:
		return nil, fmt.Errorf("cannot determine current running version for unexpected binary %s", binary)
	}
}

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
		if err := ulm.verifyExecutableInDirectory(filepath.Join(ulm.updatesDirectory(binary), versionsInLibrary[i].Original()), binary); err != nil {
			removeUpdate(versionsInLibrary[i].Original())
			continue
		}

		nonCurrentlyRunningVersionsKept += 1
	}
}

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
