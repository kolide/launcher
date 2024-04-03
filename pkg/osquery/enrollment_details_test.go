package osquery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getEnrollDetails_binaryNotExist(t *testing.T) {
	t.Parallel()

	_, err1 := getEnrollDetails(context.TODO(), filepath.Join("some", "fake", "path", "to", "osqueryd"))
	require.Error(t, err1, "expected error when path does not exist")

	_, err2 := getEnrollDetails(context.TODO(), t.TempDir())
	require.Error(t, err2, "expected error when path is directory")
}

func Test_getEnrollDetails_executionError(t *testing.T) {
	t.Parallel()

	currentExecutable, err := os.Executable()
	require.NoError(t, err, "could not get current executable for test")

	// We expect getEnrollDetails to fail when called against an executable that is not osquery
	_, err = getEnrollDetails(context.TODO(), currentExecutable)
	require.Error(t, err, "should not have been able to get enroll details with non-osqueryd executable")
}
