package packaging

import (
	"context"
	"errors"
	"fmt"
	"os"
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

	launcherPath := p.launcherLocation()

	stdout, err := p.execOut(ctx, launcherPath, "-version")
	if err != nil {
		return fmt.Errorf("failed to exec -- possibly can't autodetect while cross compiling: out `%s`: %w", stdout, err)
	}

	stdoutSplit := strings.Split(stdout, "\n")
	versionLine := strings.Split(stdoutSplit[0], " ")
	version := versionLine[len(versionLine)-1]

	if version == "" {
		return errors.New("unable to parse launcher version")
	}

	level.Debug(logger).Log("msg", "formatting version string for target platform", "origVersion", version, "platform", p.target.Platform)
	version, err = formatVersion(version, p.target.Platform)

	if err != nil {
		return fmt.Errorf("formatting version: %w", err)
	}
	level.Debug(logger).Log("msg", "successfully formatted version string", "newVersion", version)

	p.PackageVersion = version
	return nil
}

// launcherLocation returns the location of the launcher binary within `binDir`. For darwin,
// it may be in an app bundle -- we check to see if the binary exists there first, and then
// fall back to the common location if it doesn't.
func (p *PackageOptions) launcherLocation() string {
	if p.target.Platform == Darwin {
		// We want /usr/local/Kolide.app, not /usr/local/bin/Kolide.app, so we use Dir to strip out `bin`
		appBundleBinaryPath := filepath.Join(p.packageRoot, filepath.Dir(p.binDir), "Kolide.app", "Contents", "MacOS", "launcher")
		if info, err := os.Stat(appBundleBinaryPath); err == nil && !info.IsDir() {
			return appBundleBinaryPath
		}
	}

	if p.target.Platform == Windows {
		return filepath.Join(p.packageRoot, p.binDir, string(p.target.Arch), p.target.PlatformBinaryName("launcher"))
	}

	return filepath.Join(p.packageRoot, p.binDir, p.target.PlatformBinaryName("launcher"))
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
		return "", fmt.Errorf("something very wrong: expected 5 subgroups, got %d, from string %s", len(matches), rawVersion)
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
