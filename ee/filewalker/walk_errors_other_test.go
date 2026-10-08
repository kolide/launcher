//go:build !windows

package filewalker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/currentprocess"
	"github.com/stretchr/testify/require"
)

func setupExpectedWalkErrors(t *testing.T, root string) {
	if ok, err := currentprocess.IsElevated(); ok {
		t.Skip("root ignores directory permissions")
	} else if err != nil {
		require.Failf(t, "could not determine if process is privileged", "error: %+v", err)
	}

	denied := filepath.Join(root, "denied")
	require.NoError(t, os.Mkdir(denied, 0o000))
	t.Cleanup(func() { _ = os.Chmod(denied, 0o755) })
}
