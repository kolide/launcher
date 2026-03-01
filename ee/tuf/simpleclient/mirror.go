package simpleclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

// Default metadata and mirror URLs.
const (
	DefaultMetadataURL = "https://tuf.kolide.com"
	DefaultMirrorURL  = "https://dl.kolide.co"
)

// downloadAndVerify downloads the target from the mirror and verifies it against TUF metadata.
// targetPath is the full TUF path e.g. "launcher/darwin/universal/launcher-1.2.3.tar.gz".
// Returns the verified tarball bytes. All in memory.
func downloadAndVerify(ctx context.Context, mirrorURL, targetPath string, metadata data.TargetFileMeta, httpClient *http.Client) ([]byte, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	downloadPath := path.Join("/", "kolide", targetPath)
	url := strings.TrimSuffix(mirrorURL, "/") + downloadPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	stream := io.LimitReader(resp.Body, metadata.Length)
	var buf bytes.Buffer

	actualMeta, err := tufutil.GenerateTargetFileMeta(io.TeeReader(stream, &buf), metadata.HashAlgorithms()...)
	if err != nil {
		return nil, fmt.Errorf("computing hash: %w", err)
	}

	if err := tufutil.TargetFileMetaEqual(actualMeta, metadata); err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	return buf.Bytes(), nil
}
