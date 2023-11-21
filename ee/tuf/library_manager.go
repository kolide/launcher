package tuf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	mirrorUrl    string // dl.kolide.co
	mirrorClient *http.Client
	baseDir      string
	lock         *libraryLock
	logger       log.Logger
}

func newUpdateLibraryManager(mirrorUrl string, mirrorClient *http.Client, baseDir string, logger log.Logger) (*updateLibraryManager, error) {
	ulm := updateLibraryManager{
		mirrorUrl:    mirrorUrl,
		mirrorClient: mirrorClient,
		baseDir:      baseDir,
		lock:         newLibraryLock(),
		logger:       log.With(logger, "component", "tuf_autoupdater_library_manager"),
	}

	// Ensure the updates directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("could not make base directory for updates library: %w", err)
	}

	// Create the update library
	for _, binary := range binaries {
		if err := os.MkdirAll(updatesDirectory(binary, baseDir), 0755); err != nil {
			return nil, fmt.Errorf("could not make updates directory for %s: %w", binary, err)
		}
	}

	return &ulm, nil
}

// updatesDirectory returns the update library location for the given binary.
func updatesDirectory(binary autoupdatableBinary, baseUpdateDirectory string) string {
	return filepath.Join(baseUpdateDirectory, string(binary))
}

// Available determines if the given target is already available in the update library.
func (ulm *updateLibraryManager) Available(binary autoupdatableBinary, targetFilename string) bool {
	executablePath, _ := pathToTargetVersionExecutable(binary, targetFilename, ulm.baseDir)
	return autoupdate.CheckExecutable(context.TODO(), executablePath, "--version") == nil
}

// pathToTargetVersionExecutable returns the path to the executable for the desired target,
// along with its version.
func pathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string, baseUpdateDirectory string) (string, string) {
	targetVersion := versionFromTarget(binary, targetFilename)
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), targetVersion)
	return executableLocation(versionDir, binary), targetVersion
}

// AddToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there.
func (ulm *updateLibraryManager) AddToLibrary(binary autoupdatableBinary, currentVersion string, targetFilename string, targetMetadata data.TargetFileMeta) error {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	if currentVersion == versionFromTarget(binary, targetFilename) {
		return nil
	}

	if ulm.Available(binary, targetFilename) {
		return nil
	}

	stagedUpdatePath, err := ulm.stageAndVerifyUpdate(binary, targetFilename, targetMetadata)
	// Remove downloaded archives after update, regardless of success -- this will run before the unlock
	defer func() {
		if stagedUpdatePath == "" {
			return
		}
		dirToRemove := filepath.Dir(stagedUpdatePath)
		if err := os.RemoveAll(dirToRemove); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove temp staging directory", "err", err, "directory", dirToRemove)
		}
	}()
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
	stagingDir, err := ulm.tempDir(binary, fmt.Sprintf("staged-updates-%s", versionFromTarget(binary, targetFilename)))
	if err != nil {
		return "", fmt.Errorf("could not create temporary directory for downloading target: %w", err)
	}
	stagedUpdatePath := filepath.Join(stagingDir, targetFilename)

	// Request download from mirror
	downloadPath := path.Join("/", "kolide", string(binary), runtime.GOOS, PlatformArch(), targetFilename)
	resp, err := ulm.mirrorClient.Get(ulm.mirrorUrl + downloadPath)
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

// tempDir creates a directory inside of the updates directory. It is the caller's responsibility to remove
// the directory when it is no longer needed.
func (ulm *updateLibraryManager) tempDir(binary autoupdatableBinary, pattern string) (string, error) {
	directory := filepath.Join(updatesDirectory(binary, ulm.baseDir), fmt.Sprintf("%s-%d", pattern, time.Now().UnixMicro()))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("making dir %s: %w", directory, err)
	}

	return directory, nil
}

// moveVerifiedUpdate untars the update and performs final checks to make sure that it's a valid, working update.
// Finally, it moves the update to its correct versioned location in the update library for the given binary.
func (ulm *updateLibraryManager) moveVerifiedUpdate(binary autoupdatableBinary, targetFilename string, stagedUpdate string) error {
	targetVersion := versionFromTarget(binary, targetFilename)
	stagedVersionedDirectory, err := ulm.tempDir(binary, targetVersion)
	if err != nil {
		return fmt.Errorf("could not create temporary directory for untarring and validating new update: %w", err)
	}
	defer func() {
		// In case of error, clean up the staged version
		if err := os.RemoveAll(stagedVersionedDirectory); err != nil {
			level.Debug(ulm.logger).Log("msg", "could not remove staged update", "err", err, "directory", stagedVersionedDirectory)
		}
	}()

	// Untar the archive. Note that `UntarBundle` calls `filepath.Dir(destination)`, so the inclusion of `binary`
	// here doesn't matter as it's immediately stripped off.
	if err := fsutil.UntarBundle(filepath.Join(stagedVersionedDirectory, string(binary)), stagedUpdate); err != nil {
		return fmt.Errorf("could not untar update to %s: %w", stagedVersionedDirectory, err)
	}

	// Make sure that the binary is executable
	if err := os.Chmod(executableLocation(stagedVersionedDirectory, binary), 0755); err != nil {
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// If necessary, patch the executable (NixOS only)
	if err := patchExecutable(executableLocation(stagedVersionedDirectory, binary)); err != nil {
		return fmt.Errorf("could not patch executable: %w", err)
	}

	// Validate the executable
	if err := autoupdate.CheckExecutable(context.TODO(), executableLocation(stagedVersionedDirectory, binary), "--version"); err != nil {
		return fmt.Errorf("could not verify executable: %w", err)
	}

	// All good! Shelve it in the library under its version
	newUpdateDirectory := filepath.Join(updatesDirectory(binary, ulm.baseDir), targetVersion)
	if err := os.Rename(stagedVersionedDirectory, newUpdateDirectory); err != nil {
		return fmt.Errorf("could not move staged target %s from %s to %s: %w", targetFilename, stagedVersionedDirectory, newUpdateDirectory, err)
	}
	// Need rwxr-xr-x so that the desktop (running as user) can execute the downloaded binary too
	if err := os.Chmod(newUpdateDirectory, 0755); err != nil {
		return fmt.Errorf("could not chmod %s: %w", newUpdateDirectory, err)
	}

	return nil
}

// removeUpdate removes a given version from the given binary's update library.
func (ulm *updateLibraryManager) removeUpdate(binary autoupdatableBinary, binaryVersion string) {
	directoryToRemove := filepath.Join(updatesDirectory(binary, ulm.baseDir), binaryVersion)
	if err := os.RemoveAll(directoryToRemove); err != nil {
		level.Debug(ulm.logger).Log("msg", "could not remove update", "err", err, "directory", directoryToRemove)
	} else {
		level.Debug(ulm.logger).Log("msg", "removed update", "directory", directoryToRemove)
	}
}

// TidyLibrary reviews all updates in the library for the binary and removes any old versions
// that are no longer needed. It will always preserve the current running binary, and then the
// two most recent valid versions. It will remove versions it cannot validate.
func (ulm *updateLibraryManager) TidyLibrary(binary autoupdatableBinary, currentVersion string) {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	// Remove any updates we no longer need
	if currentVersion == "" {
		level.Debug(ulm.logger).Log("msg", "cannot tidy update library without knowing current running version")
		return
	}

	const numberOfVersionsToKeep = 3

	versionsInLibrary, invalidVersionsInLibrary, err := sortedVersionsInLibrary(context.Background(), binary, ulm.baseDir)
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
		if versionsInLibrary[i] == currentVersion {
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

// sortedVersionsInLibrary looks through the update library for the given binary to validate and sort all
// available versions. It returns a sorted list of the valid versions, a list of invalid versions, and
// an error only when unable to glob for versions.
func sortedVersionsInLibrary(ctx context.Context, binary autoupdatableBinary, baseUpdateDirectory string) ([]string, []string, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(updatesDirectory(binary, baseUpdateDirectory), "*"))
	if err != nil {
		return nil, nil, fmt.Errorf("could not glob for updates in library: %w", err)
	}

	versionsInLibrary := make([]*semver.Version, 0)
	invalidVersions := make([]string, 0)
	for _, rawVersionWithPath := range rawVersionsInLibrary {
		rawVersion := filepath.Base(rawVersionWithPath)
		v, err := semver.NewVersion(rawVersion)
		if err != nil {
			invalidVersions = append(invalidVersions, rawVersion)
			continue
		}

		versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), rawVersion)
		if err := autoupdate.CheckExecutable(ctx, executableLocation(versionDir, binary), "--version"); err != nil {
			traces.SetError(span, err)
			invalidVersions = append(invalidVersions, rawVersion)
			continue
		}

		// We have to swap the hyphen in the prerelease for a period (30-abcdabcd to 30.abcdabcd) so that the
		// semver library can correctly compare prerelease values.
		if v.Prerelease() != "" {
			versionWithUpdatedPrerelease, err := v.SetPrerelease(strings.Replace(v.Prerelease(), "-", ".", -1))
			if err == nil {
				v = &versionWithUpdatedPrerelease
			}
		}

		versionsInLibrary = append(versionsInLibrary, v)
	}

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Transform versions back into strings now that we've finished sorting them; swap the prerelease value back.
	versionsInLibraryStr := make([]string, len(versionsInLibrary))
	for i, v := range versionsInLibrary {
		if v.Prerelease() != "" {
			versionWithUpdatedPrerelease, err := v.SetPrerelease(strings.Replace(v.Prerelease(), ".", "-", -1))
			if err == nil {
				v = &versionWithUpdatedPrerelease
			}
		}
		versionsInLibraryStr[i] = v.Original()
	}

	return versionsInLibraryStr, invalidVersions, nil
}

// versionFromTarget extracts the semantic version for an update from its filename.
func versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}
