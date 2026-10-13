package packaging

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/v2/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/v2/pkg/launcher"
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

	// First, determine the TUF target. The TUF repo stores binaries by their name, without a file extension,
	// so strip that off first.
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	targetName, err := tufTargetPath(baseName, channelOrVersion, string(target.Platform), string(target.Arch), localCacheDir)
	if err != nil {
		return "", fmt.Errorf("getting target name: %w", err)
	}

	level.Debug(logger).Log(
		"msg", "starting download",
		"target", targetName,
	)

	// Use our own client to avoid idle connection goleaks. We have to specify a transport
	// to avoid go-tuf/v2 using the default transport.
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	defer client.CloseIdleConnections()

	// Download the package, verifying it against TUF metadata, and store it in cache
	up, tufTarget, err := lookupTarget(client, localCacheDir, targetName)
	if err != nil {
		return "", fmt.Errorf("finding target %s: %w", targetName, err)
	}
	if _, _, err := up.DownloadTarget(tufTarget, localPackagePath, launcher.DefaultMirror+"/kolide/"); err != nil {
		return "", fmt.Errorf("downloading binary archive: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(localBinaryPath), fsutil.DirMode); err != nil {
		return "", fmt.Errorf("creating directory for binary: %w", err)
	}

	// UntarBundle is a bit misnamed. this untars unto the directory
	// containing that file. It has a call to filepath.Dir(destination) there.
	if err := fsutil.UntarBundle(localBinaryPath, localPackagePath); err != nil {
		return "", fmt.Errorf("untarring download: %w", err)
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
	return channelOrVersion == "stable" || channelOrVersion == "beta" || channelOrVersion == "nightly" || channelOrVersion == "alpha"
}

func tufTargetPath(name, channelOrVersion, platform, arch, cacheDir string) (string, error) {
	// Figure out if we're downloading a specific version or a channel
	if !isChannel(channelOrVersion) {
		// We're requesting a version, not a channel, so we already know where the download lives.
		return tufTargetPathFromVersion(name, channelOrVersion, platform, arch), nil
	}

	version, err := getReleaseVersionFromTufRepo(name, channelOrVersion, platform, arch, cacheDir)
	if err != nil {
		return "", fmt.Errorf("determining release version for channel %s: %w", channelOrVersion, err)
	}

	return tufTargetPathFromVersion(name, version, platform, arch), nil
}

func tufTargetPathFromVersion(name, version, platform, arch string) string {
	return path.Join(name, platform, arch, fmt.Sprintf("%s-%s.tar.gz", name, version))
}

//go:embed assets/tuf/root.json
var rootJson []byte

func getReleaseVersionFromTufRepo(binaryName, channel, platform, arch, cacheDir string) (string, error) {
	// Use our own client to avoid idle connection goleaks. We have to specify a transport
	// to avoid go-tuf/v2 using the default transport.
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	defer client.CloseIdleConnections()

	releaseTargetName := path.Join(binaryName, platform, arch, channel, "release.json")
	_, releaseTarget, err := lookupTarget(client, cacheDir, releaseTargetName)
	if err != nil {
		return "", fmt.Errorf("finding release target %s: %w", releaseTargetName, err)
	}
	if releaseTarget.Custom == nil {
		return "", fmt.Errorf("release target %s missing custom metadata", releaseTargetName)
	}

	var custom struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(*releaseTarget.Custom, &custom); err != nil {
		return "", fmt.Errorf("unmarshalling release file custom metadata: %w", err)
	}

	targetFilename := filepath.Base(custom.Target)

	// Target looks like <binary>-<version>.tar.gz -- strip off extension and binary name to get version
	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, binaryName+"-"), ".tar.gz"), nil
}

// lookupTarget returns the target metadata for the given TUF target. It checks for metadata
// cached on disk, falling back to refreshing metadata from the TUF server. It returns the
// updater used to allow for downloading the target if needed.
func lookupTarget(client *http.Client, cacheDir string, targetName string) (*updater.Updater, *metadata.TargetFiles, error) {
	up, target, localErr := refreshAndLookupTarget(client, cacheDir, targetName, true)
	if localErr == nil {
		return up, target, nil
	}

	// Either the local metadata was unusable (missing or expired), or it was valid but did
	// not contain the requested target (e.g. a new version, channel, binary, or platform/arch
	// that postdates our cache). Either way, refresh from the server and try again.
	up, target, remoteErr := refreshAndLookupTarget(client, cacheDir, targetName, false)
	if remoteErr != nil {
		return nil, nil, fmt.Errorf("finding target via remote update after local lookup failed with `%s`: %w", localErr.Error(), remoteErr)
	}

	return up, target, nil
}

// refreshAndLookupTarget pulls in TUF metadata (from <cacheDir>/tuf only if
// localMode is set, or from the TUF server if localMode is false) and then
// looks up the given target. It returns the updater used to allow for
// downloading the target if needed. (Thus, even if localMode is false,
// an http client should always be passed in.)
func refreshAndLookupTarget(client *http.Client, cacheDir string, targetName string, localMode bool) (*updater.Updater, *metadata.TargetFiles, error) {
	tufCacheDir := filepath.Join(cacheDir, "tuf")
	// Make the directory if it doesn't already exist.
	// Ensure that directory permissions are correct, otherwise TUF will fail to initialize. We cannot
	// have permissions in excess of -rwxr-x---.
	if err := os.MkdirAll(tufCacheDir, 0750); err != nil {
		return nil, nil, fmt.Errorf("creating TUF directory inside cache dir: %w", err)
	}

	metadataUrl := strings.TrimSuffix(launcher.DefaultTufServer, "/") + "/repository"
	cfg, err := config.New(metadataUrl, rootJson)
	if err != nil {
		return nil, nil, fmt.Errorf("creating go-tuf/v2 config: %w", err)
	}
	cfg.LocalMetadataDir = tufCacheDir
	cfg.LocalTargetsDir = cacheDir
	cfg.PrefixTargetsWithHash = false // our mirror does not prefix target filenames with their hash
	cfg.UnsafeLocalMode = localMode
	if err := cfg.SetDefaultFetcherHTTPClient(client); err != nil {
		return nil, nil, fmt.Errorf("setting HTTP client on go-tuf/v2 updater: %w", err)
	}

	up, err := updater.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating go-tuf/v2 updater (local mode: %t): %w", localMode, err)
	}

	if err := up.Refresh(); err != nil {
		return nil, nil, fmt.Errorf("refreshing go-tuf/v2 updater (local mode: %t): %w", localMode, err)
	}

	target, found := up.GetTopLevelTargets()[targetName]
	if !found || target == nil {
		return nil, nil, fmt.Errorf("target %s not found in targets (local mode: %t)", targetName, localMode)
	}

	return up, target, nil
}
