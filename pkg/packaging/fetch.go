package packaging

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
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

	localBinaryPath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s", name, target.Platform, channelOrVersion), binaryName)
	localPackagePath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s.tar.gz", name, target.Platform, channelOrVersion))

	// See if a local package exists on disk already. If so, return the cached path
	if _, err := os.Stat(localBinaryPath); err == nil {
		return localBinaryPath, nil
	}

	// First, create download URI. The download mirror stores binaries by their name, without a file extension,
	// so strip that off first.
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	downloadPath, err := dlTarPath(baseName, channelOrVersion, string(target.Platform), string(target.Arch))
	if err != nil {
		return "", fmt.Errorf("could not get download path: %w", err)
	}
	url := fmt.Sprintf("https://dl.kolide.co/%s", downloadPath)

	level.Debug(logger).Log(
		"msg", "starting download",
		"url", url,
	)

	// Download the package
	downloadReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	downloadReq = downloadReq.WithContext(ctx)

	httpClient := http.DefaultClient
	response, err := httpClient.Do(downloadReq)
	if err != nil {
		return "", fmt.Errorf("couldn't download binary archive: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return "", fmt.Errorf("failed download, got http status %s", response.Status)
	}

	// Store it in cache
	writeHandle, err := os.Create(localPackagePath)
	if err != nil {
		return "", fmt.Errorf("couldn't create file handle at local package download path: %w", err)
	}
	defer writeHandle.Close()

	_, err = io.Copy(writeHandle, response.Body)
	if err != nil {
		return "", fmt.Errorf("couldn't copy HTTP response body to file: %w", err)
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

func dlTarPath(name, channelOrVersion, platform, arch string) (string, error) {
	// Figure out if we're downloading a specific version or a channel
	isChannel := channelOrVersion == "stable" || channelOrVersion == "beta" || channelOrVersion == "nightly" || channelOrVersion == "alpha"

	if !isChannel {
		// We're requesting a version, not a channel, so we already know where the download lives.
		return dlTarPathFromVersion(name, channelOrVersion, platform, arch), nil
	}

	version, err := getReleaseVersionFromTufRepo(name, channelOrVersion, platform, arch)
	if err != nil {
		return "", fmt.Errorf("could not find release version for channel %s: %w", channelOrVersion, err)
	}

	return dlTarPathFromVersion(name, version, platform, arch), nil
}

func dlTarPathFromVersion(name, version, platform, arch string) string {
	return path.Join("kolide", name, platform, arch, fmt.Sprintf("%s-%s.tar.gz", name, version))
}

//go:embed assets/tuf/root.json
var rootJson []byte

func getReleaseVersionFromTufRepo(binaryName, channel, platform, arch string) (string, error) {
	tempDir, err := os.MkdirTemp(os.TempDir(), "tuf")
	if err != nil {
		return "", fmt.Errorf("making temp TUF dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Ensure that directory permissions are correct, otherwise TUF will fail to initialize. We cannot
	// have permissions in excess of -rwxr-x---.
	if err := os.Chmod(tempDir, 0750); err != nil {
		return "", fmt.Errorf("chmodding local TUF directory %s: %w", tempDir, err)
	}

	// Set up our local store i.e. point to the directory in our filesystem
	localStore, err := filejsonstore.NewFileJSONStore(tempDir)
	if err != nil {
		return "", fmt.Errorf("could not initialize local TUF store: %w", err)
	}

	// Set up our remote store i.e. tuf.kolide.com
	remoteStore, err := client.HTTPRemoteStore("https://tuf.kolide.com", &client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}, http.DefaultClient)
	if err != nil {
		return "", fmt.Errorf("could not initialize remote TUF store: %w", err)
	}

	metadataClient := client.NewClient(localStore, remoteStore)
	if err := metadataClient.Init(rootJson); err != nil {
		return "", fmt.Errorf("failed to initialize TUF client with root JSON: %w", err)
	}

	if _, err := metadataClient.Update(); err != nil {
		return "", fmt.Errorf("failed to update metadata: %w", err)
	}

	targetToFind := path.Join(binaryName, platform, arch, channel, "release.json")
	foundTarget, err := metadataClient.Target(targetToFind)
	if err != nil {
		return "", fmt.Errorf("finding target metadata %s: %w", targetToFind, err)
	}

	var custom struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(*foundTarget.Custom, &custom); err != nil {
		return "", fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
	}

	targetFilename := filepath.Base(custom.Target)

	// Target looks like <binary>-<version>.tar.gz -- strip off extension and binary name to get version
	return strings.TrimSuffix(strings.TrimPrefix(targetFilename, binaryName+"-"), ".tar.gz"), nil
}
