package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
)

type querier interface {
	Query(query string) ([]map[string]string, error)
}

// readOnlyLibrary provides a read-only view into the updates libraries for all
// binaries. It is used to determine what version of each binary should be running.
type readOnlyLibrary struct {
	baseDir   string
	osquerier querier // used to query for current running osquery version
	logger    log.Logger
}

func newReadOnlyLibrary(baseDir string, osquerier querier, logger log.Logger) (*readOnlyLibrary, error) {
	rol := readOnlyLibrary{
		baseDir:   baseDir,
		osquerier: osquerier,
		logger:    log.With(logger, "component", "tuf_autoupdater_read_only_library"),
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

// MostRecentVersion returns the most recent, valid version available in the library for the
// given binary.
func (rol *readOnlyLibrary) MostRecentVersion(binary autoupdatableBinary) (string, error) {
	// Get current running version
	currentVersionRaw, err := rol.currentRunningVersion(binary)
	if err != nil {
		return "", fmt.Errorf("could not determine current running version of %s: %w", binary, err)
	}
	currentVersion, err := semver.NewVersion(currentVersionRaw)
	if err != nil {
		return "", fmt.Errorf("could not parse current running version %s of %s: %w", currentVersionRaw, binary, err)
	}

	// Pull all available versions from library
	validVersionsInLibrary, _, err := rol.sortedVersionsInLibrary(binary)
	if err != nil {
		return "", fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// Compare most recent version in library with current running version
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	mostRecentVersionInLibrary, err := semver.NewVersion(mostRecentVersionInLibraryRaw)
	if err != nil {
		return "", fmt.Errorf("could not parse most recent version %s in library for %s: %w", mostRecentVersionInLibraryRaw, binary, err)
	}
	if currentVersion.GreaterThan(mostRecentVersionInLibrary) {
		currentVersionExecutable, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("could not get current executable: %w", err)
		}
		return currentVersionExecutable, nil
	}

	// Update library version is more recent than current running version, so return its location
	versionDir := filepath.Join(rol.updatesDirectory(binary), mostRecentVersionInLibraryRaw)
	return executableLocation(versionDir, binary), nil
}

// PathToTargetVersionExecutable returns the path to the executable for the desired version.
func (rol *readOnlyLibrary) PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string {
	versionDir := filepath.Join(rol.updatesDirectory(binary), rol.versionFromTarget(binary, targetFilename))
	return executableLocation(versionDir, binary)
}

// Available determines if the given target is already available, either as the currently-running
// binary or within the update library.
func (rol *readOnlyLibrary) Available(binary autoupdatableBinary, targetFilename string) bool {
	// Check to see if the current running version is the version we were requested to add;
	// return early if it is, but don't error out if we can't determine the current version.
	currentVersion, err := rol.currentRunningVersion(binary)
	if err != nil {
		level.Debug(rol.logger).Log("msg", "could not get current running version", "binary", binary, "err", err)
	} else if currentVersion == rol.versionFromTarget(binary, targetFilename) {
		// We don't need to download the current running version because it already exists,
		// either in this updates library or in the original install location.
		return true
	}

	return rol.alreadyAdded(binary, targetFilename)
}

// alreadyAdded checks if the given target already exists in the update library.
func (rol *readOnlyLibrary) alreadyAdded(binary autoupdatableBinary, targetFilename string) bool {
	return autoupdate.CheckExecutable(context.TODO(), rol.PathToTargetVersionExecutable(binary, targetFilename), "--version") == nil
}

// versionFromTarget extracts the semantic version for an update from its filename.
func (rol *readOnlyLibrary) versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}

// currentRunningVersion returns the current running version of the given binary.
func (rol *readOnlyLibrary) currentRunningVersion(binary autoupdatableBinary) (string, error) {
	switch binary {
	case binaryLauncher:
		launcherVersion := version.Version().Version
		if launcherVersion == "unknown" {
			return "", errors.New("unknown launcher version")
		}
		return launcherVersion, nil
	case binaryOsqueryd:
		resp, err := rol.osquerier.Query("SELECT version FROM osquery_info;")
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
