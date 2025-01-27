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
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_getEnrollDetails_binaryNotExist(t *testing.T) {
	t.Parallel()

	var details service.EnrollmentDetails

	err1 := getOsqEnrollDetails(context.TODO(), filepath.Join("some", "fake", "path", "to", "osqueryd"), &details)
	require.Error(t, err1, "expected error when path does not exist")

	err2 := getOsqEnrollDetails(context.TODO(), t.TempDir(), &details)
	require.Error(t, err2, "expected error when path is directory")
}

func Test_getEnrollDetails_executionError(t *testing.T) {
	t.Parallel()

	var details service.EnrollmentDetails

	currentExecutable, err := os.Executable()
	require.NoError(t, err, "could not get current executable for test")

	// We expect getEnrollDetails to fail when called against an executable that is not osquery
	err = getOsqEnrollDetails(context.TODO(), currentExecutable, &details)
	require.Error(t, err, "should not have been able to get enroll details with non-osqueryd executable")
}

func TestCollectAndSetEnrollmentDetails_EmptyPath(t *testing.T) {
	t.Parallel()
	mockKnapsack := typesmocks.NewKnapsack(t)
	ctx := context.Background()
	slogger := multislogger.NewNopLogger()

	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return("")

	err := CollectAndSetEnrollmentDetails(ctx, slogger, mockKnapsack, 1*time.Second, 100*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no osqueryd path")
}

func TestCollectAndSetEnrollmentDetailsSuccess(t *testing.T) {
	t.Parallel()
	slogger := multislogger.NewNopLogger()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	testRootDir := t.TempDir()

	// Download real osqueryd binary
	target := packaging.Target{}
	require.NoError(t, target.PlatformFromString(runtime.GOOS))
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)

	osquerydPath := filepath.Join(testRootDir, target.PlatformBinaryName("osqueryd"))

	// Fetch and copy osqueryd
	downloadPath, err := packaging.FetchBinary(ctx, testRootDir, "osqueryd",
		target.PlatformBinaryName("osqueryd"), "stable", target)
	require.NoError(t, err)
	require.NoError(t, fsutil.CopyFile(downloadPath, osquerydPath))

	// Make binary executable
	require.NoError(t, os.Chmod(osquerydPath, 0755))

	mockKnapsack := typesmocks.NewKnapsack(t)

	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osquerydPath)

	detailsChan := make(chan service.EnrollmentDetails, 2) // Buffer of 2 for both calls

	mockKnapsack.On("SetEnrollmentDetails", mock.MatchedBy(func(details service.EnrollmentDetails) bool {
		detailsChan <- details
		return true
	})).Return(nil).Times(2)

	err = CollectAndSetEnrollmentDetails(ctx, slogger, mockKnapsack, 1*time.Second, 100*time.Millisecond)

	// Receive first call
	firstDetails := <-detailsChan

	// Receive second call (the one with complete details)
	finalDetails := <-detailsChan

	t.Logf("Final Details: %+v", finalDetails)

	require.NoError(t, err)
	mockKnapsack.AssertExpectations(t)

	// Runtime details
	require.NotEmpty(t, firstDetails.OSPlatform)
	require.NotEmpty(t, firstDetails.OSPlatformLike)
	require.NotEmpty(t, firstDetails.LauncherVersion)
	require.NotEmpty(t, firstDetails.GOARCH)
	require.NotEmpty(t, firstDetails.GOOS)

	// Core system details
	require.NotEmpty(t, finalDetails.OSPlatform)
	require.NotEmpty(t, finalDetails.OSName)
	require.NotEmpty(t, finalDetails.Hostname)
	require.NotEmpty(t, finalDetails.HardwareUUID)

	// Version information
	require.NotEmpty(t, finalDetails.OsqueryVersion)
	require.NotEmpty(t, finalDetails.GOOS)
	require.NotEmpty(t, finalDetails.GOARCH)
}
