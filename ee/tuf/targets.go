// Package tuf provides target resolution helpers for TUF metadata.
package tuf

import (
	"encoding/json"
	"fmt"
	"path"

	client "github.com/theupdateframework/go-tuf/client"
	"github.com/theupdateframework/go-tuf/data"
)

// ArchForPlatform returns the TUF arch string for the given platform.
// Darwin uses "universal"; others use the provided arch.
func ArchForPlatform(platform, arch string) string {
	if platform == "darwin" {
		return "universal"
	}
	return arch
}

// ResolveTarget finds the target path and metadata for the given binary, platform, arch,
// and version or channel. versionOrChannel is either a channel ("stable", "beta", etc.)
// or a specific version ("1.2.3").
// Returns the full TUF target path (e.g. "launcher/darwin/universal/launcher-1.2.3.tar.gz").
func ResolveTarget(c *client.Client, binary, platform, arch, versionOrChannel string) (targetPath string, metadata data.TargetFileMeta, err error) {
	targets, err := c.Targets()
	if err != nil {
		return "", data.TargetFileMeta{}, fmt.Errorf("getting targets: %w", err)
	}

	tufArch := ArchForPlatform(platform, arch)
	isChannel := versionOrChannel == "stable" || versionOrChannel == "beta" ||
		versionOrChannel == "nightly" || versionOrChannel == "alpha"

	if isChannel {
		return resolveTargetForChannel(targets, binary, platform, tufArch, versionOrChannel)
	}
	return resolveTargetForVersion(targets, binary, platform, tufArch, versionOrChannel)
}

func resolveTargetForChannel(targets data.TargetFiles, binary, platform, arch, channel string) (string, data.TargetFileMeta, error) {
	releaseFile := path.Join(binary, platform, arch, channel, "release.json")
	target, ok := targets[releaseFile]
	if !ok {
		return "", data.TargetFileMeta{}, fmt.Errorf("release file %s not found in targets", releaseFile)
	}

	var custom struct {
		Target string `json:"target"`
	}
	if target.Custom == nil {
		return "", data.TargetFileMeta{}, fmt.Errorf("release file %s has no custom metadata", releaseFile)
	}
	if err := json.Unmarshal(*target.Custom, &custom); err != nil {
		return "", data.TargetFileMeta{}, fmt.Errorf("parsing release metadata: %w", err)
	}

	meta, ok := targets[custom.Target]
	if !ok {
		return "", data.TargetFileMeta{}, fmt.Errorf("target %s not found in metadata", custom.Target)
	}

	return custom.Target, meta, nil
}

func resolveTargetForVersion(targets data.TargetFiles, binary, platform, arch, version string) (string, data.TargetFileMeta, error) {
	targetPath := path.Join(binary, platform, arch, fmt.Sprintf("%s-%s.tar.gz", binary, version))
	meta, ok := targets[targetPath]
	if !ok {
		return "", data.TargetFileMeta{}, fmt.Errorf("target %s not found in metadata", targetPath)
	}
	return targetPath, meta, nil
}
