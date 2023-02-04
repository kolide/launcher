package agent

import (
	"os"
	"path/filepath"
)

// TempPath returns a path within the agent's temp directory tree, by joining the path elements into a single path.
func TempPath(elem ...string) string {
	elements := append([]string{os.TempDir(), "kolide-k2"}, elem...)
	return filepath.Join(elements...)
}
