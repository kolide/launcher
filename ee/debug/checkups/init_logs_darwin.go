package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func writeInitLogs(_ context.Context, logZip *zip.Writer) error {
	stdMatches, err := filepath.Glob("/var/log/kolide-k2/*")
	if err != nil {
		return fmt.Errorf("globbing /var/log/kolide-k2/*: %w", err)
	}

	var lastErr error
	for _, f := range stdMatches {
		out, err := logZip.Create(filepath.Base(f))
		if err != nil {
			lastErr = err
			continue
		}

		in, err := os.Open(f)
		if err != nil {
			lastErr = err
			continue
		}
		defer in.Close()

		io.Copy(out, in)
	}

	return lastErr
}
