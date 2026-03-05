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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kolide/launcher/v2/ee/tuf"
	tufv2metadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufv2config "github.com/theupdateframework/go-tuf/v2/metadata/config"
	tufv2updater "github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

const (
	DefaultMetadataURL = "https://tuf.kolide.com"
	DefaultMirrorURL   = "https://dl.kolide.co"
)

// Options configures the Download and ListTargets operations.
type Options struct {
	MetadataURL string
	MirrorURL   string
	HTTPClient  *http.Client
	// LocalStorePath is the directory for the TUF local metadata store.
	// If empty, an in-memory store is used (no persistent cache).
	LocalStorePath string
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

// initUpdater creates a go-tuf v2 Updater: loads trusted root and refreshes metadata.
func initUpdater(ctx context.Context, slogger *slog.Logger, opts *Options) (*tufv2updater.Updater, error) {
	metadataBase := strings.TrimSuffix(opts.metadataURL(), "/") + "/repository"
	cfg, err := tufv2config.New(metadataBase, tuf.RootJSON())
	if err != nil {
		return nil, fmt.Errorf("creating TUF config: %w", err)
	}

	if opts.LocalStorePath != "" {
		cfg.LocalMetadataDir = filepath.Join(opts.LocalStorePath, "metadata")
		cfg.LocalTargetsDir = filepath.Join(opts.LocalStorePath, "targets")
	} else {
		cfg.DisableLocalCache = true
		cfg.LocalMetadataDir = ""
		cfg.LocalTargetsDir = ""
	}
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

// resolveTarget returns the TUF target path and metadata for the given target name,
// platform, arch, and version or channel (same semantics as ee/tuf.ResolveTarget).
func resolveTarget(up *tufv2updater.Updater, targetName, platform, arch, versionOrChannel string) (targetPath string, targetMeta *tufv2metadata.TargetFiles, err error) {
	tufArch := tuf.ArchForPlatform(platform, arch)
	isChannel := versionOrChannel == "stable" || versionOrChannel == "beta" ||
		versionOrChannel == "nightly" || versionOrChannel == "alpha"

	if isChannel {
		return resolveTargetForChannel(up, targetName, platform, tufArch, versionOrChannel)
	}
	return resolveTargetForVersion(up, targetName, platform, tufArch, versionOrChannel)
}

func resolveTargetForChannel(up *tufv2updater.Updater, targetName, platform, arch, channel string) (string, *tufv2metadata.TargetFiles, error) {
	releasePath := path.Join(targetName, platform, arch, channel, "release.json")
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

func resolveTargetForVersion(up *tufv2updater.Updater, targetName, platform, arch, version string) (string, *tufv2metadata.TargetFiles, error) {
	targetPath := path.Join(targetName, platform, arch, fmt.Sprintf("%s-%s.tar.gz", targetName, version))
	meta, err := up.GetTargetInfo(targetPath)
	if err != nil {
		return "", nil, fmt.Errorf("target %s not found: %w", targetPath, err)
	}
	return targetPath, meta, nil
}

// ListTargets fetches TUF metadata and returns the sorted list of top-level
// target names (e.g. "launcher", "osqueryd").
func ListTargets(ctx context.Context, slogger *slog.Logger, opts *Options) ([]string, error) {
	if opts == nil {
		opts = &Options{}
	}

	up, err := initUpdater(ctx, slogger, opts)
	if err != nil {
		return nil, err
	}

	targets := up.GetTopLevelTargets()
	seen := make(map[string]any)
	for p := range targets {
		name, _, _ := strings.Cut(p, "/")
		seen[name] = nil
	}

	return slices.Sorted(maps.Keys(seen)), nil
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

	target = strings.ToLower(target)

	up, err := initUpdater(ctx, slogger, opts)
	if err != nil {
		return nil, err
	}

	targetPath, targetMeta, err := resolveTarget(up, target, platform, arch, versionOrChannel)
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
