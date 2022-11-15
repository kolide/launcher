package packaging

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

// detectLauncherVersion invokes launcher and looks for the version string
func (p *PackageOptions) detectLauncherVersion(ctx context.Context) error {
	logger := log.With(ctxlog.FromContext(ctx), "library", "detectLauncherVersion")
	level.Debug(logger).Log("msg", "Attempting launcher autodetection")

	launcherPath := filepath.Join(p.packageRoot, p.binDir, p.target.PlatformBinaryName("launcher"))
	stdout, err := p.execOut(ctx, launcherPath, "-version")
	if err != nil {
		return fmt.Errorf("Failed to exec. Perhaps -- Can't autodetect while cross compiling. (%s): %w", stdout, err)
	}

	stdoutSplit := strings.Split(stdout, "\n")
	versionLine := strings.Split(stdoutSplit[0], " ")
	version := versionLine[len(versionLine)-1]

	if version == "" {
		return errors.New("Unable to parse launcher version.")
	}

	// Windows only supports a W.X.Y.Z packaing string. So we need to format this down
	if p.target.Platform == Windows {
		level.Debug(logger).Log("msg", "reformating for windows", "origVersion", version)
		version, err = formatVersion(version)
		if err != nil {
			return fmt.Errorf("formatting version: %w", err)
		}
		level.Debug(logger).Log("msg", "reformating for windows", "newVersion", version)
	}

	p.PackageVersion = version
	return nil
}

// formatVersion formats the version. This is specific to windows. It
// may show up elsewhere later.
//
// Windows packages must confirm to W.X.Y.Z, so we convert our git
// format to that.
func formatVersion(rawVersion string) (string, error) {
	versionRegex, err := regexp.Compile(`^v?(\d+)\.(\d+)(?:\.(\d+))(?:-(\d+).*)?`)
	if err != nil {
		return "", fmt.Errorf("version regex: %w", err)
	}

	// regex match and check the results
	matches := versionRegex.FindAllStringSubmatch(rawVersion, -1)

	if len(matches) == 0 {
		return "", fmt.Errorf("Version %s did not match expected format", rawVersion)
	}

	if len(matches[0]) != 5 {
		return "", fmt.Errorf("Something very wrong. Expected 5 subgroups got %d from string %s", len(matches), rawVersion)
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
