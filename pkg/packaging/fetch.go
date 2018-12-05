package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

var (
	localCacheDir string
)

func populateLocalCacheDir() error {
	tempDir, err := ioutil.TempDir("/tmp", "package-builder_cache")
	if err != nil {
		return errors.Wrap(err, "could not create temp dir for caching files")
	}
	localCacheDir = tempDir
	return nil
}

func dlTarPath(name, version, platform string) string {
	return path.Join("kolide", name, platform, fmt.Sprintf("%s-%s.tar.gz", name, version))
}

func binaryPath(osqueryVersion, osqueryPlatform string) string {
	return filepath.Join("kolide", "osqueryd", osqueryPlatform, osqueryVersion, "osqueryd")
}

// FetchOsquerydBinary will synchronously download a binary as per the
// supplied desired version and platform identifiers. The path to the
// downloaded binary is returned and an error if the operation did not
// succeed.
func FetchBinary(localCacheDir, name, version, platform string) (string, error) {
	// Create the cache directory if it doesn't already exist
	if localCacheDir == "" {
		if err := populateLocalCacheDir(); err != nil {
			return "", errors.Wrap(err, "could not create local cache directory")
		}
	}

	localBinaryPath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s", name, platform, version), name)
	localPackagePath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s.tar.gz", name, platform, version))

	// See if a local package exists on disk already. If so, return the cached path
	if _, err := os.Stat(localBinaryPath); err == nil {
		return localBinaryPath, nil
	}

	// If not we have to download the package. First, create download URI
	url := fmt.Sprintf("https://dl.kolide.co/%s", dlTarPath(name, version, platform))

	// Download the package
	response, err := http.Get(url)
	if err != nil {
		return "", errors.Wrap(err, "couldn't download binary archive")
	}
	defer response.Body.Close()

	// Store it in cache
	writeHandle, err := os.Create(localPackagePath)
	if err != nil {
		return "", errors.Wrap(err, "couldn't create file handle at local package download path")
	}
	defer writeHandle.Close()

	_, err = io.Copy(writeHandle, response.Body)
	if err != nil {
		return "", errors.Wrap(err, "couldn't copy HTTP response body to file")
	}

	// explicitly close the write handle before untaring the archive
	writeHandle.Close()

	if err := os.MkdirAll(filepath.Dir(localBinaryPath), fs.DirMode); err != nil {
		return "", errors.Wrap(err, "couldn't create directory for binary")
	}

	if err := fs.UntarBundle(localBinaryPath, localPackagePath); err != nil {
		return "", errors.Wrap(err, "couldn't untar package")
	}

	if _, err := os.Stat(localBinaryPath); err != nil {
		return "", errors.Wrap(err, "local binary does not exist but it should")
	}

	return localBinaryPath, nil
}
