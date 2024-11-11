package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"path/filepath"
)

func writeInitLogs(_ context.Context, logZip *zip.Writer) error {
	stdMatches, err := filepath.Glob("/var/log/kolide-k2/*")
	if err != nil {
		return fmt.Errorf("globbing /var/log/kolide-k2/*: %w", err)
	}

	var lastErr error
	for _, f := range stdMatches {
		if err := addFileToZip(logZip, f); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
