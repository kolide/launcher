package packaging

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// detectLauncherVersion invokes launcher and looks for the version string
func (p *PackageOptions) detectLauncherVersion(ctx context.Context) error {
	launcherPath := filepath.Join(p.packageRoot, p.binDir, p.target.PlatformBinaryName("launcher"))
	stdout, err := p.execOut(ctx, launcherPath, "-version")
	if err != nil {
		return errors.Wrapf(err, "Failed to exec. Perhaps -- Can't autodetect while cross compiling. (%s)", stdout)
	}

	stdoutSplit := strings.Split(stdout, "\n")
	versionLine := strings.Split(stdoutSplit[0], " ")
	version := versionLine[len(versionLine)-1]

	if version == "" {
		return errors.New("Unable to parse launcher version.")
	}

	formattedVersion, err := formatVersion(version)
	if err != nil {
		return errors.Wrap(err, "formatting version")
	}

	p.PackageVersion = formattedVersion
	return nil
}
