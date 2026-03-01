// Package simpleclient provides a minimal TUF client for downloading and verifying
// targets from the TUF repository. It handles metadata fetching, target resolution,
// and verified download, returning the verified bytes.
package simpleclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/tuf"
	client "github.com/theupdateframework/go-tuf/client"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

const (
	DefaultMetadataURL = "https://tuf.kolide.com"
	DefaultMirrorURL   = "https://dl.kolide.co"
)

// Options configures the Download operation.
type Options struct {
	MetadataURL string
	MirrorURL   string
	HTTPClient  *http.Client
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

// Download fetches a target from the TUF mirror and verifies it against TUF metadata.
// Returns the verified tarball bytes.
//
// target is the TUF target name (e.g. "launcher", "osqueryd").
// platform must be "darwin", "linux", or "windows".
// arch must be "amd64" or "arm64" (darwin uses "universal" automatically).
// versionOrChannel is a channel ("stable", "beta", etc.) or specific version ("1.2.3").
func Download(ctx context.Context, slogger *slog.Logger, target, platform, arch, versionOrChannel string, opts *Options) ([]byte, error) {
	if opts == nil {
		opts = &Options{}
	}
	httpClient := opts.httpClient()

	target = strings.ToLower(target)

	// Create an in-memory TUF metadata client and update
	metadataStart := time.Now()
	localStore := client.MemoryLocalStore()
	remoteStore, err := client.HTTPRemoteStore(opts.metadataURL(), &client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}, httpClient)
	if err != nil {
		return nil, fmt.Errorf("creating remote store: %w", err)
	}
	metadataClient := client.NewClient(localStore, remoteStore)

	if err := metadataClient.Init(tuf.RootJSON()); err != nil {
		return nil, fmt.Errorf("initializing TUF client: %w", err)
	}
	if _, err := metadataClient.Update(); err != nil {
		return nil, fmt.Errorf("updating TUF metadata: %w", err)
	}

	// Resolve target name + channel/version to a concrete target path
	targetPath, metadata, err := tuf.ResolveTarget(metadataClient, target, platform, arch, versionOrChannel)
	if err != nil {
		return nil, fmt.Errorf("resolving target: %w", err)
	}
	slogger.Log(ctx, slog.LevelDebug,
		"TUF metadata updated and target resolved",
		"target_path", targetPath,
		"duration", time.Since(metadataStart).String(),
	)

	// Download from mirror
	downloadStart := time.Now()
	downloadURL := strings.TrimSuffix(opts.mirrorURL(), "/") + path.Join("/", "kolide", targetPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
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

	// Read and verify against TUF metadata
	var buf bytes.Buffer
	stream := io.LimitReader(resp.Body, metadata.Length)
	actualMeta, err := tufutil.GenerateTargetFileMeta(io.TeeReader(stream, &buf), metadata.HashAlgorithms()...)
	if err != nil {
		return nil, fmt.Errorf("computing hash: %w", err)
	}
	if err := tufutil.TargetFileMetaEqual(actualMeta, metadata); err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}
	slogger.Log(ctx, slog.LevelDebug,
		"target downloaded and verified",
		"target_path", targetPath,
		"size", buf.Len(),
		"duration", time.Since(downloadStart).String(),
	)

	return buf.Bytes(), nil
}
