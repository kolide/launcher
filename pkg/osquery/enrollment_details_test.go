package osquery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/kit/fsutil"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/service"
	"github.com/stretchr/testify/mock"
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

func TestCollectAndSetEnrollmentDetails(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	// Download real osqueryd binary
	target := packaging.Target{}
	require.NoError(t, target.PlatformFromString(runtime.GOOS))
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	osquerydPath := filepath.Join(testRootDir, target.PlatformBinaryName("osqueryd"))

	// Fetch and copy osqueryd
	downloadPath, err := packaging.FetchBinary(context.TODO(), testRootDir, "osqueryd",
		target.PlatformBinaryName("osqueryd"), "stable", target)
	require.NoError(t, err)
	require.NoError(t, fsutil.CopyFile(downloadPath, osquerydPath))

	// Make binary executable
	require.NoError(t, os.Chmod(osquerydPath, 0755))

	t.Run("empty osqueryd path", func(t *testing.T) {
		t.Parallel()
		mockKnapsack := typesmocks.NewKnapsack(t)
		ctx := context.Background()

		mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return("")

		err := CollectAndSetEnrollmentDetails(ctx, mockKnapsack, 1*time.Second, 100*time.Millisecond)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no osqueryd path")
	})

	t.Run("successful collection", func(t *testing.T) {
		t.Parallel()
		mockKnapsack := typesmocks.NewKnapsack(t)
		ctx := context.Background()

		mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osquerydPath)
		mockKnapsack.On("SetEnrollmentDetails", mock.Anything).Return(nil)

		err := CollectAndSetEnrollmentDetails(ctx, mockKnapsack, 100*time.Millisecond, 100*time.Millisecond)
		require.NoError(t, err)
		mockKnapsack.AssertExpectations(t)
	})

	t.Run("collection timeout", func(t *testing.T) {
		t.Parallel()
		mockKnapsack := typesmocks.NewKnapsack(t)
		ctx := context.Background()

		mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return("/usr/local/bin/osqueryd")

		err := CollectAndSetEnrollmentDetails(ctx, mockKnapsack, 100*time.Millisecond, 100*time.Millisecond)
		require.Error(t, err)
		require.Contains(t, err.Error(), "collecting enrollment details")
	})
}
