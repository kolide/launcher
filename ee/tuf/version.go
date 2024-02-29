package tuf

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	tufVersionMinimum            = semver.MustParse("1.4.1")
	pinnedLauncherVersionMinimum = semver.MustParse("1.6.1")
)

// SanitizePinnedVersion ensures that the given version is a valid semantic version,
// and that it meets the requirements for pinning.
func SanitizePinnedVersion(binary autoupdatableBinary, rawVersion string) string {
	parsedVersion, err := semver.NewVersion(rawVersion)
	if err != nil {
		// Invalid semver
		return ""
	}

	// For osqueryd, we will accept any valid semver -- the autoupdater will validate
	// that the version exists at update time.
	if binary != binaryLauncher {
		return rawVersion
	}

	// For launcher, we require that the version is at least greater than v1.6.1, the
	// first version to support pinning versions.
	if parsedVersion.LessThan(pinnedLauncherVersionMinimum) {
		return ""
	}
	return rawVersion
}

// launcherVersionSupportsTuf determines if the given version is greater than the minimum
// required to run the new autoupdater.
func launcherVersionSupportsTuf(rawVersion string) (bool, error) {
	versionParsed, err := semver.NewVersion(rawVersion)
	if err != nil {
		return false, fmt.Errorf("could not parse launcher version %s: %w", rawVersion, err)
	}

	return versionParsed.GreaterThan(tufVersionMinimum) || versionParsed.Equal(tufVersionMinimum), nil
}

// versionFromTarget extracts the semantic version for an update from its filename.
func versionFromTarget(binary autoupdatableBinary, targetFilename string) string {
	// The target is in the form `launcher-0.13.6.tar.gz` -- trim the prefix and the file extension to return the version
	prefixToTrim := fmt.Sprintf("%s-", binary)

	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, prefixToTrim), ".tar.gz")
}
