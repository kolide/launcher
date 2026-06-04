package packaging

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/v2/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/v2/pkg/packagekit"
)

// sanitizeHostname will replace any ":" characters in a given hostname with "-"
// This is useful because ":" is not a valid character for file paths.
func sanitizeHostname(hostname string) string {
	return strings.ReplaceAll(hostname, ":", "-")
}

// setPackageVersion retrieves the launcher version from TUF if it's not already provided,
// and sets it as the package version.
func (p *PackageOptions) setPackageVersion(launcherChannelOrVersion string, localCacheDir string) error {
	launcherVersion, err := p.getFormattedVersion("launcher", launcherChannelOrVersion, localCacheDir)
	if err != nil {
		return fmt.Errorf("determining package version: %w", err)
	}
	p.PackageVersion = launcherVersion
	return nil
}

// setOsqueryVersionInCtx retrieves the osquery version from TUF if it's not already provided,
// and sets it in the context.
func (p *PackageOptions) setOsqueryVersionInCtx(ctx context.Context, osquerydChannelOrVersion string, localCacheDir string) {
	logger := log.With(ctxlog.FromContext(ctx), "library", "setOsqueryVersionInCtx")

	osquerydVersion, err := p.getFormattedVersion("osqueryd", osquerydChannelOrVersion, localCacheDir)
	if err != nil {
		level.Warn(logger).Log(
			"msg", "could not get osqueryd version",
			"version", osquerydChannelOrVersion,
			"err", err,
		)
		return
	}

	packagekit.SetInContext(ctx, packagekit.ContextOsqueryVersionKey, osquerydVersion)
}

func (p *PackageOptions) getFormattedVersion(binary string, channelOrVersion string, localCacheDir string) (string, error) {
	versionRaw := channelOrVersion
	if isChannel(channelOrVersion) {
		versionFromTuf, err := getReleaseVersionFromTufRepo(binary, channelOrVersion, string(p.target.Platform), string(p.target.Arch), localCacheDir)
		if err != nil {
			return "", fmt.Errorf("getting release version for %s from TUF for channel %s: %w", binary, channelOrVersion, err)
		}
		versionRaw = versionFromTuf
	}

	versionFormatted, err := formatVersion(versionRaw, p.target.Platform)
	if err != nil {
		return "", fmt.Errorf("%s version %s (determined from %s) cannot be formatted as package version: %w", binary, versionRaw, channelOrVersion, err)
	}

	return versionFormatted, nil
}

// formatVersion formats the version according to the packaging requirements of the target platform
// currently, only windows and darwin platforms require modification
func formatVersion(rawVersion string, platform PlatformFlavor) (string, error) {
	if platform != Windows && platform != Darwin {
		return rawVersion, nil
	}

	versionRegex, err := regexp.Compile(`^v?(\d+)\.(\d+)(?:\.(\d+))(?:-(\d+).*)?`)
	if err != nil {
		return "", fmt.Errorf("version regex: %w", err)
	}

	// regex match and check the results
	matches := versionRegex.FindAllStringSubmatch(rawVersion, -1)

	if len(matches) == 0 {
		return "", fmt.Errorf("version %s did not match expected format", rawVersion)
	}

	if len(matches[0]) != 5 {
		return "", fmt.Errorf("something very wrong: expected 5 subgroups, got %d, from string %s", len(matches[0]), rawVersion)
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

	switch platform {
	case Windows:
		// Windows expects a <major>.<minor>.<patch>.<commit> packaging string
		return fmt.Sprintf("%s.%s.%s.%s", major, minor, patch, commits), nil
	case Darwin:
		// Darwin expects a <major>.<minor>.<patch> packaging string
		return fmt.Sprintf("%s.%s.%s", major, minor, patch), nil
	case Linux:
		return rawVersion, nil
	default:
		return "", fmt.Errorf("unsupported platform %v", platform)
	}
}
