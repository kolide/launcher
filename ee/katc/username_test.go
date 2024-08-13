package katc

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_addUsernameFromFilePath(t *testing.T) {
	t.Parallel()

	// Create a path with an expected home directory
	homeDirs := homeDirLocations[runtime.GOOS]
	expectedUsername := "test-user"
	firstRow := make(map[string][]byte)
	sourcePath := filepath.Join(homeDirs[0], expectedUsername, "some", "path", "to", "db.sqlite")
	result, err := addUsernameFromFilePath(context.TODO(), multislogger.NewNopLogger(), sourcePath, firstRow)
	require.NoError(t, err)
	require.Contains(t, result, "username")
	require.Equal(t, expectedUsername, string(result["username"]))

	// Create a path without an expected home directory
	otherSourcePath := filepath.Join("some", "other", "path", "to", "db.sqlite")
	secondRow := make(map[string][]byte)
	resultWithoutUsername, err := addUsernameFromFilePath(context.TODO(), multislogger.NewNopLogger(), otherSourcePath, secondRow)
	require.NoError(t, err)
	require.NotContains(t, resultWithoutUsername, "username")
}
