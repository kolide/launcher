package osquery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/pkg/service"
	"github.com/stretchr/testify/require"
)

func Test_getEnrollDetails_binaryNotExist(t *testing.T) {
	t.Parallel()

	var details service.EnrollmentDetails

	err1 := GetOsqEnrollDetails(context.TODO(), filepath.Join("some", "fake", "path", "to", "osqueryd"), &details)
	require.Error(t, err1, "expected error when path does not exist")

	err2 := GetOsqEnrollDetails(context.TODO(), t.TempDir(), &details)
	require.Error(t, err2, "expected error when path is directory")
}

func Test_getEnrollDetails_executionError(t *testing.T) {
	t.Parallel()

	var details service.EnrollmentDetails

	currentExecutable, err := os.Executable()
	require.NoError(t, err, "could not get current executable for test")

	// We expect getEnrollDetails to fail when called against an executable that is not osquery
	err = GetOsqEnrollDetails(context.TODO(), currentExecutable, &details)
	require.Error(t, err, "should not have been able to get enroll details with non-osqueryd executable")
}
