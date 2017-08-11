package packaging

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

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

func osqueryTarPath(osqueryVersion, osqueryPlatform string) string {
	return filepath.Join("kolide", "osqueryd", osqueryPlatform, fmt.Sprintf("osqueryd-%s.tar.gz", osqueryVersion))
}

func osqueryBinaryPath(osqueryVersion, osqueryPlatform string) string {
	return filepath.Join("kolide", "osqueryd", osqueryPlatform, osqueryVersion, "osqueryd")
}

// FetchOsquerydBinary will synchronously download an osquery binary as per the
// supplied desired osquery version and platform identifiers. The path to the
// downloaded binary is returned and an error if the operation did not succeed.
func FetchOsquerydBinary(osqueryVersion, osqueryPlatform string) (string, error) {
	// Create the cache directory if it doesn't already exist
	if localCacheDir == "" {
		if err := populateLocalCacheDir(); err != nil {
			return "", errors.Wrap(err, "could not create local cache directory")
		}
	}

	// See if a local package exists on disk already. If so, return the cached path
	localBinaryDownloadPath := filepath.Join(localCacheDir, osqueryBinaryPath(osqueryVersion, osqueryPlatform))
	if _, err := os.Stat(localBinaryDownloadPath); err == nil {
		return localBinaryDownloadPath, nil
	}

	// If not we have to download the package. First, create download URI
	url := fmt.Sprintf("https://dl.kolide.com/%s", osqueryTarPath(osqueryVersion, osqueryPlatform))

	// Download the package
	localPackageDownloadPath := filepath.Join(localCacheDir, osqueryTarPath(osqueryVersion, osqueryPlatform))
	if err := os.MkdirAll(filepath.Dir(localPackageDownloadPath), DirMode); err != nil {
		return "", errors.Wrap(err, "couldn't create directory for package")
	}

	response, err := http.Get(url)
	if err != nil {
		return "", errors.Wrap(err, "couldn't download osquery binary archive")
	}
	defer response.Body.Close()

	// Store it in cache
	writeHandle, err := os.Create(localPackageDownloadPath)
	if err != nil {
		return "", errors.Wrap(err, "couldn't create file handle at local package download path")
	}
	defer writeHandle.Close()

	_, err = io.Copy(writeHandle, response.Body)
	if err != nil {
		return "", errors.Wrap(err, "couldn't copy HTTP response body to file")
	}

	// Untar the file
	untarHandle, err := os.Open(localPackageDownloadPath)
	if err != nil {
		return "", errors.Wrap(err, "couldn't create read file handle for untar process")
	}
	defer untarHandle.Close()

	if err := os.MkdirAll(filepath.Dir(localBinaryDownloadPath), DirMode); err != nil {
		return "", errors.Wrap(err, "couldn't create directory for binary")
	}

	if err := Untar(filepath.Dir(localBinaryDownloadPath), untarHandle); err != nil {
		return "", errors.Wrap(err, "could not untar the package")
	}

	if _, err := os.Stat(localBinaryDownloadPath); err != nil {
		return "", errors.Wrap(err, "local binary does not exist but it should")
	}

	return localBinaryDownloadPath, nil
}
