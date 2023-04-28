package tuf

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/theupdateframework/go-tuf/data"
)

// findRelease checks the latest data from TUF (in `targets`) to see whether a new release
// has been published for the given channel. If it has, it returns the target for that release
// and its associated metadata.
func findRelease(binary autoupdatableBinary, targets data.TargetFiles, channel string) (string, data.TargetFileMeta, error) {
	// First, find the target that the channel release file is pointing to
	var releaseTarget string
	targetReleaseFile := fmt.Sprintf(genericReleaseVersionFormat, binary, runtime.GOOS, channel)
	for targetName, target := range targets {
		if targetName != targetReleaseFile {
			continue
		}

		// We found the release file that matches our OS and binary. Evaluate it
		// to see if we're on this latest version.
		var custom ReleaseFileCustomMetadata
		if err := json.Unmarshal(*target.Custom, &custom); err != nil {
			return "", data.TargetFileMeta{}, fmt.Errorf("could not unmarshal release file custom metadata: %w", err)
		}

		releaseTarget = custom.Target
		break
	}

	if releaseTarget == "" {
		return "", data.TargetFileMeta{}, fmt.Errorf("expected release file %s for binary %s to be in targets but it was not", targetReleaseFile, binary)
	}

	// Now, get the metadata for our release target
	for targetName, target := range targets {
		if targetName != releaseTarget {
			continue
		}

		return filepath.Base(releaseTarget), target, nil
	}

	return "", data.TargetFileMeta{}, fmt.Errorf("could not find metadata for release target %s for binary %s", targetReleaseFile, binary)
}
