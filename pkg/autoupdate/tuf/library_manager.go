package tuf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

var launcherVersionRegex = regexp.MustCompile(`launcher - version (\d+\.\d+\.\d+(?:-.+)?)\n`)

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	mirrorUrl    string // dl.kolide.co
	mirrorClient *http.Client
	baseDir      string
	stagingDir   string
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

	// Create the directory for staging updates
	stagingDir, err := agent.MkdirTemp("staged-updates")
	if err != nil {
		return nil, fmt.Errorf("could not make staged updates directory: %w", err)
	}
	ulm.stagingDir = stagingDir

	// Create the update library
	for _, binary := range binaries {
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

// Available determines if the given target is already available in the update library.
func (ulm *updateLibraryManager) Available(binary autoupdatableBinary, targetFilename string) bool {
	executablePath := ulm.PathToTargetVersionExecutable(binary, targetFilename)
	return autoupdate.CheckExecutable(context.TODO(), executablePath, "--version") == nil
}

// PathToTargetVersionExecutable returns the path to the executable for the desired version.
func (ulm *updateLibraryManager) PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string {
	versionDir := filepath.Join(ulm.updatesDirectory(binary), versionFromTarget(binary, targetFilename))
	return executableLocation(versionDir, binary)
}

// AddToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there.
func (ulm *updateLibraryManager) AddToLibrary(binary autoupdatableBinary, targetFilename string, targetMetadata data.TargetFileMeta) error {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	if ulm.IsInstallVersion(binary, targetFilename) {
		return nil
	}

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
	targetVersion := versionFromTarget(binary, targetFilename)
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
func (ulm *updateLibraryManager) TidyLibrary(binary autoupdatableBinary, currentVersion string) {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	// First, remove old staged archives
	ulm.tidyStagedUpdates(binary)

	// Remove any updates we no longer need
	ulm.tidyUpdateLibrary(binary, currentVersion)
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

// sortedVersionsInLibrary looks through the update library for the given binary to validate and sort all
// available versions. It returns a sorted list of the valid versions, a list of invalid versions, and
// an error only when unable to glob for versions.
func (ulm *updateLibraryManager) sortedVersionsInLibrary(binary autoupdatableBinary) ([]string, []string, error) {
	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(ulm.updatesDirectory(binary), "*"))
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

		versionDir := filepath.Join(ulm.updatesDirectory(binary), rawVersion)
		if err := autoupdate.CheckExecutable(context.TODO(), executableLocation(versionDir, binary), "--version"); err != nil {
			invalidVersions = append(invalidVersions, rawVersion)
			continue
		}

		versionsInLibrary = append(versionsInLibrary, v)
	}

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Transform versions back into strings now that we've finished sorting them
	versionsInLibraryStr := make([]string, len(versionsInLibrary))
	for i, v := range versionsInLibrary {
		versionsInLibraryStr[i] = v.Original()
	}

	return versionsInLibraryStr, invalidVersions, nil
}

// IsInstallVersion checks whether the version in the given target is the same as the one
// contained in the installation.
func (ulm *updateLibraryManager) IsInstallVersion(binary autoupdatableBinary, targetFilename string) bool {
	installedVersion, _, err := ulm.installedVersion(binary)
	if err != nil {
		level.Debug(ulm.logger).Log("msg", "could not get installed version", "binary", binary, "err", err)
		return false
	}

	return installedVersion.Original() == versionFromTarget(binary, targetFilename)
}

// installedVersion returns the version of, and the path to, the originally-installed binary.
func (ulm *updateLibraryManager) installedVersion(binary autoupdatableBinary) (*semver.Version, string, error) {
	pathToBinary := ulm.findInstallLocation(binary)
	if pathToBinary == "" {
		return nil, "", fmt.Errorf("could not find install location for `%s`", binary)
	}

	if cachedVersion, err := ulm.getCachedInstalledVersion(binary); err == nil {
		return cachedVersion, pathToBinary, nil
	}

	cmd := exec.Command(pathToBinary, "--version")
	cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE") // Prevents launcher from fork-bombing
	out, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("could not execute %s --version: out %s, error %w", pathToBinary, string(out), err)
	}

	var v *semver.Version
	switch binary {
	case binaryLauncher:
		v, err = parseLauncherVersion(out)
	case binaryOsqueryd:
		v, err = parseOsquerydVersion(out)
	default:
		return nil, "", fmt.Errorf("cannot parse version for unknown binary %s", binary)
	}

	if err != nil {
		return nil, "", fmt.Errorf("could not parse binary install version: %w", err)
	}

	ulm.cacheInstalledVersion(binary, v)

	return v, pathToBinary, nil
}

// findInstallLocation attempts to find the install location for the given binary, looking
// in well-known locations.
func (ulm *updateLibraryManager) findInstallLocation(binary autoupdatableBinary) string {
	binaryName := string(binary)
	if runtime.GOOS == "windows" {
		binaryName = binaryName + ".exe"
	}

	// Places that we expect to see binaries installed
	var likelyPaths []string

	if binary == binaryOsqueryd {
		likelyPaths = append(likelyPaths, executableLocation(`C:\Program Files\osquery`, binary))
	}

	likelyPaths = append(
		likelyPaths,
		executableLocation("/usr/local/kolide", binary),
		executableLocation("/usr/local/kolide/bin", binary),
		executableLocation("/usr/local/kolide-k2", binary),
		executableLocation("/usr/local/kolide-k2/bin", binary),
		executableLocation("/usr/local/bin", binary),
		executableLocation(`C:\Program Files\Kolide\Launcher-kolide-k2\bin`, binary),
	)

	// We want to check the current executable path last, since that may pick up updates instead
	if currentExecutablePath, err := os.Executable(); err == nil {
		likelyPaths = append(likelyPaths, filepath.Join(filepath.Dir(currentExecutablePath), binaryName))
	}

	for _, potentialPath := range likelyPaths {
		potentialPath = filepath.Clean(potentialPath)

		info, err := os.Stat(potentialPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		return potentialPath
	}

	// If we haven't found it anywhere else, look for it on the PATH
	if potentialPath, err := exec.LookPath(binaryName); err == nil {
		return potentialPath
	}

	level.Debug(ulm.logger).Log(
		"msg", "could not find install location in any well-known paths",
		"binary", binaryName,
		"likely_paths", fmt.Sprintf("%+v", likelyPaths),
	)

	return ""
}

// getCachedInstalledVersion reads the install version from a cached file in the updates directory.
func (ulm *updateLibraryManager) getCachedInstalledVersion(binary autoupdatableBinary) (*semver.Version, error) {
	versionBytes, err := os.ReadFile(ulm.cachedInstalledVersionLocation(binary))
	if err != nil {
		return nil, fmt.Errorf("could not read cached installed version file: %w", err)
	}

	v, err := semver.NewVersion(string(versionBytes))
	if err != nil {
		return nil, fmt.Errorf("could not parse cached installed version file: %w", err)
	}

	return v, nil
}

// cacheInstalledVersion caches the install version in a file in the updates directory, to avoid
// having to exec the binary to discover its version every time.
func (ulm *updateLibraryManager) cacheInstalledVersion(binary autoupdatableBinary, installedVersion *semver.Version) {
	if err := os.WriteFile(ulm.cachedInstalledVersionLocation(binary), []byte(installedVersion.Original()), 0644); err != nil {
		level.Debug(ulm.logger).Log("msg", "could not cache installed version", "binary", binary, "err", err)
	}
}

// cachedInstalledVersionLocation returns the location of the cached install version file.
func (ulm *updateLibraryManager) cachedInstalledVersionLocation(binary autoupdatableBinary) string {
	return filepath.Join(ulm.baseDir, fmt.Sprintf("%s-installed-version", binary))
}

// versionFromTarget extracts the semantic version for an update from its filename.
func versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

// parseLauncherVersion parses the launcher version from the output of `launcher --version`.
func parseLauncherVersion(versionOutput []byte) (*semver.Version, error) {
	matches := launcherVersionRegex.FindStringSubmatch(string(versionOutput))
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not parse launcher version from output %s", string(versionOutput))
	}
	launcherInstallVersion, err := semver.NewVersion(matches[1])
	if err != nil {
		return nil, fmt.Errorf("could not parse launcher version %s as semver: %w", matches[1], err)
	}

	return launcherInstallVersion, nil
}

// parseOsquerydVersion parses the osqueryd version from the output of `osqueryd --version`.
func parseOsquerydVersion(versionOutput []byte) (*semver.Version, error) {
	versionTrimmed := strings.TrimPrefix(strings.TrimPrefix(string(versionOutput), "osqueryd version"), "osqueryd.exe version")
	versionStr := strings.TrimSpace(versionTrimmed)
	osqueryInstallVersion, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse osquery version `%s` as semver: %w", versionStr, err)
	}

	return osqueryInstallVersion, nil
}
