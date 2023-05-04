package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
)

// readOnlyLibrary provides a read-only view into the updates libraries for all
// binaries. It is used to determine what version of each binary should be running.
type readOnlyLibrary struct {
	baseDir string
	logger  log.Logger
}

func newReadOnlyLibrary(baseDir string, logger log.Logger) (*readOnlyLibrary, error) {
	rol := readOnlyLibrary{
		baseDir: baseDir,
		logger:  log.With(logger, "component", "tuf_autoupdater_read_only_library"),
	}

	// Ensure the updates directory exists
	if _, err := os.Stat(baseDir); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("updates directory %s does not exist: %w", baseDir, err)
	}

	// Ensure the individual libraries exist
	for _, binary := range binaries {
		if _, err := os.Stat(rol.updatesDirectory(binary)); errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("update library for %s does not exist: %w", binary, err)
		}
	}

	return &rol, nil
}

// updatesDirectory returns the update library location for the given binary.
func (rol *readOnlyLibrary) updatesDirectory(binary autoupdatableBinary) string {
	return filepath.Join(rol.baseDir, string(binary))
}

// MostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary. If the installed version is the most recent version, it returns an empty string.
func (rol *readOnlyLibrary) MostRecentVersion(binary autoupdatableBinary) (string, error) {
	// Get installed version
	installedVersion, installedVersionPath, err := InstalledVersion(binary)
	if err != nil {
		return "", fmt.Errorf("could not determine current running version of %s: %w", binary, err)
	}

	// Pull all available versions from library
	validVersionsInLibrary, _, err := rol.sortedVersionsInLibrary(binary)
	if err != nil {
		return "", fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// If we don't have any updates in the library, return the installed version
	if len(validVersionsInLibrary) < 1 {
		return installedVersionPath, nil
	}

	// Compare most recent version in library with the installed version
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	mostRecentVersionInLibrary, err := semver.NewVersion(mostRecentVersionInLibraryRaw)
	if err != nil {
		return "", fmt.Errorf("could not parse most recent version %s in library for %s: %w", mostRecentVersionInLibraryRaw, binary, err)
	}
	if installedVersion.GreaterThan(mostRecentVersionInLibrary) {
		return "", nil
	}

	// The update library version is more recent than the installed version, so return its location
	versionDir := filepath.Join(rol.updatesDirectory(binary), mostRecentVersionInLibraryRaw)
	return executableLocation(versionDir, binary), nil
}

// PathToTargetVersionExecutable returns the path to the executable for the desired version.
func (rol *readOnlyLibrary) PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string {
	versionDir := filepath.Join(rol.updatesDirectory(binary), versionFromTarget(binary, targetFilename))
	return executableLocation(versionDir, binary)
}

// IsInstallVersion checks whether the version in the given target is the same as the one
// contained in the installation.
func (rol *readOnlyLibrary) IsInstallVersion(binary autoupdatableBinary, targetFilename string) bool {
	installedVersion, _, err := InstalledVersion(binary)
	if err != nil {
		level.Debug(rol.logger).Log("msg", "could not get installed version", "binary", binary, "err", err)
		return false
	}

	return installedVersion.Original() == versionFromTarget(binary, targetFilename)
}

// Available determines if the given target is already available in the update library.
func (rol *readOnlyLibrary) Available(binary autoupdatableBinary, targetFilename string) bool {
	executablePath := rol.PathToTargetVersionExecutable(binary, targetFilename)
	return autoupdate.CheckExecutable(context.TODO(), executablePath, "--version") == nil
}

// sortedVersionsInLibrary looks through the update library for the given binary to validate and sort all
// available versions. It returns a sorted list of the valid versions, a list of invalid versions, and
// an error only when unable to glob for versions.
func (rol *readOnlyLibrary) sortedVersionsInLibrary(binary autoupdatableBinary) ([]string, []string, error) {
	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(rol.updatesDirectory(binary), "*"))
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

		versionDir := filepath.Join(rol.updatesDirectory(binary), rawVersion)
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

// versionFromTarget extracts the semantic version for an update from its filename.
func versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

func InstalledVersion(binary autoupdatableBinary) (*semver.Version, string, error) {
	// TODO cache it somewhere
	pathToBinary := findInstallLocation(binary)
	if pathToBinary == "" {
		return nil, "", errors.New("could not find install location")
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

	return v, pathToBinary, nil
}

// parseLauncherVersion parses the launcher version from the output of `launcher --version`.
func parseLauncherVersion(versionOutput []byte) (*semver.Version, error) {
	// TODO: trim everything that's not line `launcher - version 1.0.7-19-g8c890f3`
	versionStr := strings.TrimSpace(strings.TrimPrefix("launcher - version", string(versionOutput)))
	launcherInstallVersion, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse launcher version %s as semver: %w", versionStr, err)
	}

	return launcherInstallVersion, nil
}

// parseOsquerydVersion parses the osqueryd version from the output of `osqueryd --version`.
func parseOsquerydVersion(versionOutput []byte) (*semver.Version, error) {
	versionStr := strings.TrimSpace(strings.TrimPrefix("osqueryd version", string(versionOutput)))
	osqueryInstallVersion, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse osquery version %s as semver: %w", versionStr, err)
	}

	return osqueryInstallVersion, nil
}

// TODO ripped directly from findOsquery()
func findInstallLocation(binary autoupdatableBinary) string {
	binaryName := string(binary)
	if runtime.GOOS == "windows" {
		binaryName = binaryName + ".exe"
	}

	var likelyDirectories []string

	if exPath, err := os.Executable(); err == nil {
		likelyDirectories = append(likelyDirectories, filepath.Dir(exPath))
	}

	// Places to check. We could conditionalize on GOOS, but it doesn't
	// seem important.
	likelyDirectories = append(
		likelyDirectories,
		"/usr/local/kolide/bin",
		"/usr/local/kolide-k2/bin",
		"/usr/local/bin",
		`C:\Program Files\osquery`,
		`C:\Program Files\Kolide\Launcher-kolide-k2\bin`,
	)

	for _, dir := range likelyDirectories {
		potentialPath := filepath.Join(filepath.Clean(dir), binaryName)

		info, err := os.Stat(potentialPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		// I guess it's good enough...
		return potentialPath
	}

	// last ditch, check for binary on the PATH
	if osqPath, err := exec.LookPath(binaryName); err == nil {
		return osqPath
	}

	return ""
}
