package agent

import (
	"os"
	"path/filepath"
)

// TempPath returns a path within the agent's temp directory tree, by joining the path elements into a single path.
func TempPath(elem ...string) string {
	elements := append([]string{os.TempDir()}, elem...)
	return filepath.Join(elements...)
}

// MkdirTemp creates a new temporary directory in the TempPath directory.
func MkdirTemp(pattern string) (string, error) {
	return os.MkdirTemp(TempPath(), pattern)
}
