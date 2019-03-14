// +build windows

package packaging

import (
	"fmt"
	"regexp"

	"github.com/pkg/errors"
)

// formatVersion formats the version. This is specific to windows. It
// may show up elsewhere later.
//
// Windows packages must confirm to W.X.Y.Z, so we convert our git
// format to that.
func formatVersion(rawVersion string) (string, error) {
	versionRegex, err := regexp.Compile(`^v?(\d+)\.(\d+)(?:\.(\d+))(?:-(.+))?`)
	if err != nil {
		return "", errors.Wrap(err, "version regex")
	}

	// regex match and check the results
	matches := versionRegex.FindAllStringSubmatch(rawVersion, -1)

	if len(matches) == 0 {
		return "", errors.Errorf("Version %s did not match expected format", rawVersion)
	}

	if len(matches[0]) != 5 {
		return "", errors.Errorf("Something very wrong. Expected 5 subgroups got %d from string %s", len(matches), rawVersion)
	}

	major := matches[0][1]
	minor := matches[0][2]
	patch := matches[0][3]
	commits := matches[0][4]

	// If things are "", they should be 0
	if major == "" {
		major = "0"
	}
	if minor == "" {
		minor = "0"
	}
	if patch == "" {
		patch = "0"
	}
	if commits == "" {
		commits = "0"
	}

	version := fmt.Sprintf("%s.%s.%s.%s", major, minor, patch, commits)

	return version, nil
}
