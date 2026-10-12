package packaging

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/v2/ee/tuf"
	"github.com/kolide/launcher/v2/ee/tuf/simpleclient"
	"github.com/kolide/launcher/v2/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// FetchBinary will synchronously download a binary as per the
// supplied desired version and platform identifiers. The path to the
// downloaded binary is returned or an error if the operation did not
// succeed.
//
// You must specify a localCacheDir, to reuse downloads
func FetchBinary(ctx context.Context, localCacheDir, name, binaryName, channelOrVersion string, target Target) (string, error) {
	logger := ctxlog.FromContext(ctx)

	// Create the cache directory if it doesn't already exist
	if localCacheDir == "" {
		return "", errors.New("empty cache dir argument")
	}

	// put binaries in arch specific directory for Windows to avoid naming collisions in wix msi building
	// where a single destination will have multiple, mutally exclusive sources
	if target.Platform == Windows {
		localCacheDir = filepath.Join(localCacheDir, string(target.Arch))
	}

	if err := os.MkdirAll(localCacheDir, fsutil.DirMode); err != nil {
		return "", fmt.Errorf("could not create cache directory: %w", err)
	}

	localBinaryPath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s-%s", name, target.Platform, target.Arch, channelOrVersion), binaryName)
	localPackagePath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s-%s.tar.gz", name, target.Platform, target.Arch, channelOrVersion))

	// See if a local package exists on disk already. If so, return the cached path
	if _, err := os.Stat(localBinaryPath); err == nil {
		return localBinaryPath, nil
	}

	tarGzBytes, err := simpleclient.Download(ctx, multislogger.NewNopLogger(), name, string(target.Platform), string(target.Arch), channelOrVersion, nil)
	if err != nil {
		return "", fmt.Errorf("error fetching %s: %w", name, err)
	}

	// Store it in cache
	writeHandle, err := os.Create(localPackagePath)
	if err != nil {
		return "", fmt.Errorf("couldn't create file handle at local package download path: %w", err)
	}
	defer writeHandle.Close()

	if _, err := writeHandle.Write(tarGzBytes); err != nil {
		return "", fmt.Errorf("couldn't copy response to file: %w", err)
	}

	// explicitly close the write handle before untaring the archive
	writeHandle.Close()

	if err := os.MkdirAll(filepath.Dir(localBinaryPath), fsutil.DirMode); err != nil {
		return "", fmt.Errorf("couldn't create directory for binary: %w", err)
	}

	// UntarBundle is a bit misnamed. this untars unto the directory
	// containing that file. It has a call to filepath.Dir(destination) there.
	if err := fsutil.UntarBundle(localBinaryPath, localPackagePath); err != nil {
		return "", fmt.Errorf("couldn't untar download: %w", err)
	}

	if _, err := os.Stat(localBinaryPath); err != nil {
		level.Debug(logger).Log(
			"msg", "Missing local binary",
			"localBinaryPath", localBinaryPath,
		)
		return "", fmt.Errorf("local binary does not exist but it should: %w", err)
	}

	return localBinaryPath, nil
}

func isChannel(channelOrVersion string) bool {
	lowercaseChannelOrVersion := strings.ToLower(channelOrVersion)
	return lowercaseChannelOrVersion == "stable" || lowercaseChannelOrVersion == "beta" || lowercaseChannelOrVersion == "nightly" || lowercaseChannelOrVersion == "alpha"
}

func getReleaseVersionFromTufRepo(binaryName, channel, platform, arch, cacheDir string) (string, error) {
	tufCacheDir := filepath.Join(cacheDir, "tuf")
	// Make the directory if it doesn't already exist.
	// Ensure that directory permissions are correct, otherwise TUF will fail to initialize. We cannot
	// have permissions in excess of -rwxr-x---.
	if err := os.MkdirAll(tufCacheDir, 0750); err != nil {
		return "", fmt.Errorf("creating TUF directory inside cache dir: %w", err)
	}

	metadataUrl := strings.TrimSuffix(launcher.DefaultTufServer, "/") + "/repository"
	cfg, err := config.New(metadataUrl, tuf.RootJSON())
	if err != nil {
		return "", fmt.Errorf("creating go-tuf/v2 config: %w", err)
	}
	cfg.LocalMetadataDir = tufCacheDir
	cfg.LocalTargetsDir = cacheDir
	cfg.UnsafeLocalMode = true

	releaseTargetName := path.Join(binaryName, platform, arch, channel, "release.json")

	// First, try to find the release target using only cached metadata on disk.
	releaseTarget, localErr := lookupReleaseTarget(cfg, releaseTargetName)
	if localErr != nil {
		// Either the local metadata was unusable (missing or expired), or it was valid but did
		// not contain the requested target (e.g. a new channel, binary, or platform/arch that
		// postdates our cache). In both cases, refresh from the server and look again.
		cfg.UnsafeLocalMode = false
		var remoteErr error
		releaseTarget, remoteErr = lookupReleaseTarget(cfg, releaseTargetName)
		if remoteErr != nil {
			return "", fmt.Errorf("finding release target via remote update after local lookup failed with `%s`: %w", localErr.Error(), remoteErr)
		}
	}

	var custom struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(*releaseTarget.Custom, &custom); err != nil {
		return "", fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
	}

	targetFilename := filepath.Base(custom.Target)

	// Target looks like <binary>-<version>.tar.gz -- strip off extension and binary name to get version
	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, binaryName+"-"), ".tar.gz"), nil
}

func lookupReleaseTarget(cfg *config.UpdaterConfig, releaseTargetName string) (*metadata.TargetFiles, error) {
	up, err := updater.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating go-tuf/v2 updater (local mode: %t): %w", cfg.UnsafeLocalMode, err)
	}

	if err := up.Refresh(); err != nil {
		return nil, fmt.Errorf("refreshing go-tuf/v2 updater (local mode: %t): %w", cfg.UnsafeLocalMode, err)
	}

	releaseTarget, found := up.GetTopLevelTargets()[releaseTargetName]
	if !found || releaseTarget == nil || releaseTarget.Custom == nil {
		return nil, fmt.Errorf("release target %s not found in targets (local mode: %t)", releaseTargetName, cfg.UnsafeLocalMode)
	}

	return releaseTarget, nil
}
