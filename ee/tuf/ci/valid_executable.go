package tufci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func CopyBinary(t *testing.T, executablePath string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))

	require.NoError(t, os.Symlink(os.Args[0], executablePath))
}
