package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"path/filepath"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

func writeInitLogs(_ context.Context, k types.Knapsack, logZip *zip.Writer) error {
	logGlob := fmt.Sprintf("/var/log/%s/*", k.Identifier())
	stdMatches, err := filepath.Glob(logGlob)
	if err != nil {
		return fmt.Errorf("globbing %s: %w", logGlob, err)
	}

	var lastErr error
	for _, f := range stdMatches {
		if err := addFileToZip(logZip, f); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
