// Package simpleclient provides a minimal TUF client for downloading and verifying
// targets from the TUF repository. It uses go-tuf v2 and is isolated from the rest
// of the codebase, which continues to use go-tuf v0.x.
package simpleclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/kolide/launcher/v2/ee/tuf"
	"github.com/kolide/launcher/v2/pkg/launcher"
	tufv2metadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufv2config "github.com/theupdateframework/go-tuf/v2/metadata/config"
	tufv2updater "github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// Options configures the Download and ListTargets operations.
type Options struct {
	MetadataURL string
	MirrorURL   string
	HTTPClient  *http.Client
}

func (o *Options) metadataURL() string {
	if o.MetadataURL != "" {
		return o.MetadataURL
	}
	return launcher.DefaultTufServer
}

func (o *Options) mirrorURL() string {
	if o.MirrorURL != "" {
		return o.MirrorURL
	}
	return launcher.DefaultMirror
}

func (o *Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// newUpdater creates a go-tuf v2 Updater: loads trusted root and refreshes metadata.
// Metadata is kept in memory only (no persistent local cache), which avoids the need
// for write access to a local TUF store.
func newUpdater(ctx context.Context, slogger *slog.Logger, opts *Options) (*tufv2updater.Updater, error) {
	metadataBase := strings.TrimSuffix(opts.metadataURL(), "/") + "/repository"
	cfg, err := tufv2config.New(metadataBase, tuf.RootJSON())
	if err != nil {
		return nil, fmt.Errorf("creating TUF config: %w", err)
	}

	cfg.DisableLocalCache = true
	cfg.LocalMetadataDir = ""
	cfg.LocalTargetsDir = ""
	cfg.PrefixTargetsWithHash = false

	if err := cfg.SetDefaultFetcherHTTPClient(opts.httpClient()); err != nil {
		return nil, fmt.Errorf("setting HTTP client: %w", err)
	}

	up, err := tufv2updater.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating TUF updater: %w", err)
	}

	start := time.Now()
	if err := up.Refresh(); err != nil {
		return nil, fmt.Errorf("refreshing TUF metadata: %w", err)
	}
	slogger.Log(ctx, slog.LevelDebug,
		"TUF metadata updated",
		"duration", time.Since(start).String(),
	)

	return up, nil
}

// resolveTarget returns the fully-qualified TUF target path and metadata for the given
// binary (e.g. "launcher", "osqueryd"), platform, arch, and version or channel.
func resolveTarget(up *tufv2updater.Updater, binary, platform, arch, versionOrChannel string) (targetPath string, targetMeta *tufv2metadata.TargetFiles, err error) {
	tufArch := tuf.ArchForPlatform(platform, arch)
	isChannel := versionOrChannel == "stable" || versionOrChannel == "beta" ||
		versionOrChannel == "nightly" || versionOrChannel == "alpha"

	if isChannel {
		return resolveTargetForChannel(up, binary, platform, tufArch, versionOrChannel)
	}
	return resolveTargetForVersion(up, binary, platform, tufArch, versionOrChannel)
}

func resolveTargetForChannel(up *tufv2updater.Updater, binary, platform, arch, channel string) (string, *tufv2metadata.TargetFiles, error) {
	releasePath := path.Join(binary, platform, arch, channel, "release.json")
	releaseMeta, err := up.GetTargetInfo(releasePath)
	if err != nil {
		return "", nil, fmt.Errorf("release file %s not found: %w", releasePath, err)
	}

	var custom struct {
		Target string `json:"target"`
	}
	if releaseMeta.Custom == nil {
		return "", nil, fmt.Errorf("release file %s has no custom metadata", releasePath)
	}
	if err := json.Unmarshal(*releaseMeta.Custom, &custom); err != nil {
		return "", nil, fmt.Errorf("parsing release metadata: %w", err)
	}

	meta, err := up.GetTargetInfo(custom.Target)
	if err != nil {
		return "", nil, fmt.Errorf("target %s not found: %w", custom.Target, err)
	}

	return custom.Target, meta, nil
}

func resolveTargetForVersion(up *tufv2updater.Updater, binary, platform, arch, version string) (string, *tufv2metadata.TargetFiles, error) {
	targetPath := path.Join(binary, platform, arch, fmt.Sprintf("%s-%s.tar.gz", binary, version))
	meta, err := up.GetTargetInfo(targetPath)
	if err != nil {
		return "", nil, fmt.Errorf("target %s not found: %w", targetPath, err)
	}
	return targetPath, meta, nil
}

// ListTargets fetches TUF metadata and returns the sorted list of distinct binary
// names (e.g. "launcher", "osqueryd"). go-tuf reports every fully-qualified target
// path (e.g. "launcher/darwin/universal/launcher-1.2.3.tar.gz"); we derive the binary
// name from the first path segment and deduplicate.
func ListTargets(ctx context.Context, slogger *slog.Logger, opts *Options) ([]string, error) {
	if opts == nil {
		opts = &Options{}
	}

	up, err := newUpdater(ctx, slogger, opts)
	if err != nil {
		return nil, fmt.Errorf("creating TUF updater: %w", err)
	}

	seen := make(map[string]any)
	for targetPath := range up.GetTopLevelTargets() {
		binary, _, _ := strings.Cut(targetPath, "/")
		seen[binary] = nil
	}

	return slices.Sorted(maps.Keys(seen)), nil
}

// Download fetches a binary from the TUF mirror and verifies it against TUF metadata.
// Returns the verified tarball bytes.
//
// binary is the binary name (e.g. "launcher", "osqueryd").
// platform must be "darwin", "linux", or "windows".
// arch must be "amd64" or "arm64" (darwin uses "universal" automatically).
// versionOrChannel is a channel ("stable", "beta", etc.) or specific version ("1.2.3").
func Download(ctx context.Context, slogger *slog.Logger, binary, platform, arch, versionOrChannel string, opts *Options) ([]byte, error) {
	if opts == nil {
		opts = &Options{}
	}

	binary = strings.ToLower(binary)

	up, err := newUpdater(ctx, slogger, opts)
	if err != nil {
		return nil, fmt.Errorf("creating TUF updater: %w", err)
	}

	targetPath, targetMeta, err := resolveTarget(up, binary, platform, arch, versionOrChannel)
	if err != nil {
		return nil, fmt.Errorf("resolving target: %w", err)
	}
	slogger.Log(ctx, slog.LevelDebug,
		"target resolved",
		"target_path", targetPath,
	)

	downloadStart := time.Now()
	targetBaseURL := strings.TrimSuffix(opts.mirrorURL(), "/") + "/kolide/"
	_, data, err := up.DownloadTarget(targetMeta, "", targetBaseURL)
	if err != nil {
		return nil, fmt.Errorf("downloading target: %w", err)
	}

	slogger.Log(ctx, slog.LevelDebug,
		"target downloaded and verified",
		"target_path", targetPath,
		"size", len(data),
		"duration", time.Since(downloadStart).String(),
	)

	return data, nil
}
