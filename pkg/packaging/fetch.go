package packaging

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// FetchOsquerydBinary will synchronously download a binary as per the
// supplied desired version and platform identifiers. The path to the
// downloaded binary is returned or an error if the operation did not
// succeed.
//
// You must specify a localCacheDir, to reuse downloads
func FetchBinary(ctx context.Context, localCacheDir, name, version, platform string) (string, error) {
	logger := ctxlog.FromContext(ctx)

	// Create the cache directory if it doesn't already exist
	if localCacheDir == "" {
		return "", errors.New("Empty cache dir argument")
	}

	localBinaryPath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s", name, platform, version), name)
	localPackagePath := filepath.Join(localCacheDir, fmt.Sprintf("%s-%s-%s.tar.gz", name, platform, version))

	// See if a local package exists on disk already. If so, return the cached path
	if _, err := os.Stat(localBinaryPath); err == nil {
		return localBinaryPath, nil
	}

	// If not we have to download the package. First, create download
	// URI. Notary stores things by name, sans extension. So just strip
	// it off.
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	url := fmt.Sprintf("https://dl.kolide.co/%s", dlTarPath(baseName, version, platform))

	level.Debug(logger).Log(
		"msg", "starting download",
		"url", url,
	)

	// Download the package
	downloadReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", errors.Wrap(err, "new request")
	}
	downloadReq = downloadReq.WithContext(ctx)

	httpClient := http.DefaultClient
	response, err := httpClient.Do(downloadReq)
	if err != nil {
		return "", errors.Wrap(err, "couldn't download binary archive")
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return "", errors.Errorf("Failed download. Got http status %s", response.Status)
	}

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

	// UntarBundle is a bit misnamed. this untars unto the directory
	// containing that file. It has a call to filepath.Dir(destination) there.
	if err := fs.UntarBundle(localBinaryPath, localPackagePath); err != nil {
		return "", errors.Wrap(err, "couldn't untar download")
	}

	// TODO / FIXME this fails for osquery-extension, since the binary name is inconsistent.
	if _, err := os.Stat(localBinaryPath); err != nil {
		return "", errors.Wrap(err, "local binary does not exist but it should")
	}

	return localBinaryPath, nil
}

func dlTarPath(name, version, platform string) string {
	return path.Join("kolide", name, platform, fmt.Sprintf("%s-%s.tar.gz", name, version))
}
