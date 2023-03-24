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

	ulm.tidyLibrary(binary)

	return nil
}

func (ulm *updateLibraryManager) alreadyAdded(binary string, targetFilename string) bool {
	updateLocation := filepath.Join(ulm.updatesDirectory(binary), ulm.versionFromTarget(binary, targetFilename), targetFilename)

	return ulm.verifyExecutable(updateLocation) == nil
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
	outputBinary := filepath.Join(newUpdateDirectory, binary)
	if ulm.operatingSystem == "windows" {
		outputBinary += ".exe"
	}
	if err := os.Chmod(outputBinary, 0755); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// Validate the executable
	if err := ulm.verifyExecutable(outputBinary); err != nil {
		removeBrokenUpdateDir()
		return fmt.Errorf("could not verify executable: %w", err)
	}

	return nil
}

func (ulm *updateLibraryManager) verifyExecutable(filepath string) error {
	stat, err := os.Stat(filepath)
	switch {
	case os.IsNotExist(err):
		// Target has not been downloaded
		return fmt.Errorf("file does not exist at %s", filepath)
	case stat.IsDir():
		return fmt.Errorf("expected executable but got directory at %s", filepath)
	case err != nil:
		// Can't check -- assume it's not added
		return fmt.Errorf("error checking file info for %s: %w", filepath, err)
	case stat.Mode()&0111 == 0:
		// Exists but not executable
		return fmt.Errorf("file %s is not executable", filepath)
	}

	// TODO run with --version

	return nil
}

func (ulm *updateLibraryManager) tidyLibrary(binary string) {
	// Get the version for the currently-running launcher so we can preserve it
	currentVersion, err := semver.NewVersion(version.Version().Version)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "cannot parse current running version, cannot safely tidy updates library", "err", err)
	}

	const numberOfVersionsToKeep = 3

	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(ulm.updatesDirectory(binary), "*"))
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not glob for updates to tidy updates library", "err", err)
		return
	}

	if len(rawVersionsInLibrary) <= numberOfVersionsToKeep {
		return
	}

	versionsInLibrary := make([]*semver.Version, 0)
	for _, rawVersion := range rawVersionsInLibrary {
		v, err := semver.NewVersion(rawVersion)
		if err != nil {
			level.Debug(ulm.logger).Log("msg", "updates library contains invalid semver", "err", err, "library_version", rawVersion)
			// TODO possibly should remove this?
			continue
		}

		if v == currentVersion {
			// Don't add the current version to our list of versions slated for removal
			continue
		}

		versionsInLibrary = append(versionsInLibrary, v)
	}

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Remove all but the most recent ones
	for i := 0; i < len(versionsInLibrary)-numberOfVersionsToKeep; i += 1 {
		directoryToRemove := filepath.Join(ulm.updatesDirectory(binary), versionsInLibrary[i].Original())
		if err := os.RemoveAll(directoryToRemove); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove old update when tidying updates library", "err", err, "directory", directoryToRemove)
		}
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
