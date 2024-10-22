package packaging

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit"
)

// sanitizeHostname will replace any ":" characters in a given hostname with "-"
// This is useful because ":" is not a valid character for file paths.
func sanitizeHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
}

// setOsqueryVersionInCtx retrieves the osquery version (by running the binary) and
// sets it in the context.
func (p *PackageOptions) setOsqueryVersionInCtx(ctx context.Context) {
	logger := log.With(ctxlog.FromContext(ctx), "library", "setOsqueryVersionInCtx")

	osqueryPath := p.osqueryLocation()
	stdout, err := p.execOut(ctx, osqueryPath, "-version")
	if err != nil {
		level.Warn(logger).Log(
			"msg", "could not run osqueryd -version",
			"err", err,
		)
		return
	}

	packagekit.SetInContext(ctx, packagekit.ContextOsqueryVersionKey, osqueryVersionFromVersionOutput(stdout))
}

func osqueryVersionFromVersionOutput(output string) string {
	// Output looks like `osquery version x.y.z`, so split on `version` and return the last part of the string
	parts := strings.SplitAfter(output, "version")
	return strings.TrimSpace(parts[len(parts)-1])
}

// osqueryLocation returns the location of the osquery binary within `binDir`. For darwin,
// it may be in an app bundle -- we check to see if the binary exists there first, and then
// fall back to the common location if it doesn't.
func (p *PackageOptions) osqueryLocation() string {
	if p.target.Platform == Darwin {
		// We want /usr/local/osquery.app, not /usr/local/bin/Kolide.app, so we use Dir to strip out `bin`
		appBundleBinaryPath := filepath.Join(p.packageRoot, filepath.Dir(p.binDir), "osquery.app", "Contents", "MacOS", "launcher")
		if info, err := os.Stat(appBundleBinaryPath); err == nil && !info.IsDir() {
			return appBundleBinaryPath
		}
	}

	if p.target.Platform == Windows {
		return filepath.Join(p.packageRoot, p.binDir, string(p.target.Arch), p.target.PlatformBinaryName("osqueryd"))
	}

	return filepath.Join(p.packageRoot, p.binDir, p.target.PlatformBinaryName("osqueryd"))
}
