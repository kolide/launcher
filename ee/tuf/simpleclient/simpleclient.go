// Package simpleclient provides a minimal TUF client for downloading and verifying
// launcher and osqueryd binaries. It uses ee/tuf for metadata and verification,
// keeping the flow in memory with a single disk write for the output binary.
package simpleclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/tuf"
	client "github.com/theupdateframework/go-tuf/client"
)

// newMetadataClient creates a TUF metadata client using an in-memory store.
// It fetches metadata from the given URL. Call Init(rootJson) and Update() before use.
func newMetadataClient(metadataURL string, httpClient *http.Client) (*client.Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	localStore := client.MemoryLocalStore()
	remoteStore, err := client.HTTPRemoteStore(metadataURL, &client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}, httpClient)
	if err != nil {
		return nil, fmt.Errorf("creating remote store: %w", err)
	}
	return client.NewClient(localStore, remoteStore), nil
}

// Options configures the Download operation.
type Options struct {
	// MetadataURL is the TUF metadata server (default: https://tuf.kolide.com).
	MetadataURL string
	// MirrorURL is the mirror for binary downloads (default: https://dl.kolide.co).
	MirrorURL string
	// HTTPClient is used for HTTP requests. If nil, uses default with 2min timeout.
	HTTPClient *http.Client
}

func (o *Options) metadataURL() string {
	if o.MetadataURL != "" {
		return o.MetadataURL
	}
	return DefaultMetadataURL
}

func (o *Options) mirrorURL() string {
	if o.MirrorURL != "" {
		return o.MirrorURL
	}
	return DefaultMirrorURL
}

func (o *Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// Download fetches the binary from the TUF mirror, verifies it against TUF metadata,
// and streams it to dest. All in memory except the write to dest.
//
// binary must be "launcher" or "osqueryd".
// platform must be "darwin", "linux", or "windows".
// arch must be "amd64" or "arm64" (darwin uses "universal" automatically).
// versionOrChannel is a channel ("stable", "beta", etc.) or specific version ("1.2.3").
func Download(ctx context.Context, binary, platform, arch, versionOrChannel string, dest io.Writer, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}

	binary = strings.ToLower(binary)
	if binary != "launcher" && binary != "osqueryd" {
		return fmt.Errorf("binary must be launcher or osqueryd, got %q", binary)
	}

	// 1. Create metadata client and fetch
	metadataClient, err := newMetadataClient(opts.metadataURL(), opts.httpClient())
	if err != nil {
		return fmt.Errorf("creating metadata client: %w", err)
	}

	if err := metadataClient.Init(tuf.RootJSON()); err != nil {
		return fmt.Errorf("initializing TUF client: %w", err)
	}

	if _, err := metadataClient.Update(); err != nil {
		return fmt.Errorf("updating TUF metadata: %w", err)
	}

	// 2. Resolve target
	targetPath, metadata, err := tuf.ResolveTarget(metadataClient, binary, platform, arch, versionOrChannel)
	if err != nil {
		return fmt.Errorf("resolving target: %w", err)
	}

	// 3. Download and verify (in memory)
	tarGzBytes, err := downloadAndVerify(ctx, opts.mirrorURL(), targetPath, metadata, opts.httpClient())
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	// 4. Extract binary from tarball (in memory)
	binaryPath := binaryPathInTarball(platform, binary)
	binaryBytes, err := extractBinaryFromTarGz(tarGzBytes, binaryPath)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// 5. Stream to dest
	if _, err := dest.Write(binaryBytes); err != nil {
		return fmt.Errorf("writing binary: %w", err)
	}

	return nil
}
