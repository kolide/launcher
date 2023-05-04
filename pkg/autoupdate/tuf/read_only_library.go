package tuf

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/kolide/launcher/pkg/autoupdate"
)

// readOnlyLibrary provides a read-only view into the updates libraries for all
// binaries.
type readOnlyLibrary struct {
	baseDir string
	logger  log.Logger
}

var launcherVersionRegex = regexp.MustCompile(`launcher - version (\d+\.\d+\.\d+(?:-.+)?)\n`)

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

// PathToTargetVersionExecutable returns the path to the executable for the desired version.
func (rol *readOnlyLibrary) PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string {
	versionDir := filepath.Join(rol.updatesDirectory(binary), versionFromTarget(binary, targetFilename))
	return executableLocation(versionDir, binary)
}

// IsInstallVersion checks whether the version in the given target is the same as the one
// contained in the installation.
func (rol *readOnlyLibrary) IsInstallVersion(binary autoupdatableBinary, targetFilename string) bool {
	installedVersion, _, err := rol.installedVersion(binary)
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

// installedVersion returns the version of, and the path to, the originally-installed binary.
func (rol *readOnlyLibrary) installedVersion(binary autoupdatableBinary) (*semver.Version, string, error) {
	pathToBinary := findInstallLocation(binary)
	if pathToBinary == "" {
		return nil, "", fmt.Errorf("could not find install location for `%s`", binary)
	}

	if cachedVersion, err := rol.getCachedInstalledVersion(binary); err == nil {
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

	rol.cacheInstalledVersion(binary, v)

	return v, pathToBinary, nil
}

// getCachedInstalledVersion reads the install version from a cached file in the updates directory.
func (rol *readOnlyLibrary) getCachedInstalledVersion(binary autoupdatableBinary) (*semver.Version, error) {
	versionBytes, err := os.ReadFile(rol.cachedInstalledVersionLocation(binary))
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
func (rol *readOnlyLibrary) cacheInstalledVersion(binary autoupdatableBinary, installedVersion *semver.Version) {
	if err := os.WriteFile(rol.cachedInstalledVersionLocation(binary), []byte(installedVersion.Original()), 0644); err != nil {
		level.Debug(rol.logger).Log("msg", "could not cache installed version", "binary", binary, "err", err)
	}
}

// cachedInstalledVersionLocation returns the location of the cached install version file.
func (rol *readOnlyLibrary) cachedInstalledVersionLocation(binary autoupdatableBinary) string {
	return filepath.Join(rol.baseDir, fmt.Sprintf("%s-installed-version", binary))
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
	versionStr := strings.TrimSpace(strings.TrimPrefix(string(versionOutput), "osqueryd version"))
	osqueryInstallVersion, err := semver.NewVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("could not parse osquery version `%s` as semver: %w", versionStr, err)
	}

	return osqueryInstallVersion, nil
}

// findInstallLocation attempts to find the install location for the given binary, looking
// in well-known locations.
func findInstallLocation(binary autoupdatableBinary) string {
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
		likelyPaths = append(likelyPaths, filepath.Join(filepath.Dir(currentExecutablePath), string(binary)))
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

	return ""
}
