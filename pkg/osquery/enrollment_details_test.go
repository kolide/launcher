package osquery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
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
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	testRootDir := t.TempDir()
	slogger.InfoContext(ctx, "created test directory", "path", testRootDir)

	// Download real osqueryd binary
	target := packaging.Target{}
	require.NoError(t, target.PlatformFromString(runtime.GOOS))
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	osquerydPath := filepath.Join(testRootDir, target.PlatformBinaryName("osqueryd"))

	// Fetch and copy osqueryd
	downloadPath, err := packaging.FetchBinary(ctx, testRootDir, "osqueryd",
		target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		t.Fatalf("failed to fetch binary: %v\nLogs:\n%s", err, logBytes.String())
	}
	slogger.InfoContext(ctx, "downloaded osqueryd", "download_path", downloadPath)
	require.NoError(t, err)
	require.NoError(t, fsutil.CopyFile(downloadPath, osquerydPath))

	// Make binary executable
	if err := os.Chmod(osquerydPath, 0755); err != nil {
		t.Fatalf("failed to chmod binary: %v\nLogs:\n%s", err, logBytes.String())
	}
	slogger.InfoContext(ctx, "made binary executable")

	detailsChan := make(chan types.EnrollmentDetails, 2)
	var detailsCount int32
	expectedDetails := 2

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osquerydPath)

	// First call expectation - Runtime details
	mockKnapsack.On("SetEnrollmentDetails", mock.MatchedBy(func(details types.EnrollmentDetails) bool {
		return details.LauncherVersion != "" && details.OsqueryVersion == ""
	})).Run(func(args mock.Arguments) {
		details := args.Get(0).(types.EnrollmentDetails)
		slogger.InfoContext(ctx, "first SetEnrollmentDetails match attempt", "details", details)
		detailsChan <- details
		atomic.AddInt32(&detailsCount, 1)
	}).Return(nil).Once()

	// Second call expectation - Full details with osquery data
	mockKnapsack.On("SetEnrollmentDetails", mock.MatchedBy(func(details types.EnrollmentDetails) bool {
		return details.LauncherVersion != "" && details.OsqueryVersion != ""
	})).Run(func(args mock.Arguments) {
		details := args.Get(0).(types.EnrollmentDetails)
		slogger.InfoContext(ctx, "second SetEnrollmentDetails match attempt", "details", details)
		detailsChan <- details
		atomic.AddInt32(&detailsCount, 1)
	}).Return(nil).Once()

	testDone := make(chan struct{})
	go func() {
		defer close(testDone)

		slogger.InfoContext(ctx, "starting CollectAndSetEnrollmentDetails")
		err = CollectAndSetEnrollmentDetails(ctx, slogger, mockKnapsack, 30*time.Second, 5*time.Second)
		if err != nil {
			slogger.ErrorContext(ctx, "CollectAndSetEnrollmentDetails failed", "error", err)
			return
		}

		// Wait for all details to be processed with timeout
		deadline := time.After(30 * time.Second)
		for atomic.LoadInt32(&detailsCount) < int32(expectedDetails) {
			select {
			case <-deadline:
				t.Error("timeout waiting for enrollment details")
				return
			case <-time.After(5 * time.Second):
				slogger.DebugContext(ctx, "waiting for details", "current_count", atomic.LoadInt32(&detailsCount))
				continue
			}
		}
	}()

	select {
	case <-testDone:
		require.Equal(t, int32(expectedDetails), atomic.LoadInt32(&detailsCount))
	case <-time.After(45 * time.Second):
		t.Fatalf("test timed out\nLogs:\n%s", logBytes.String())
	}

	// Get and verify the details
	firstDetails := <-detailsChan
	slogger.InfoContext(ctx, "received first details", "details", firstDetails)

	finalDetails := <-detailsChan
	slogger.InfoContext(ctx, "received final details", "details", finalDetails)

	t.Logf("Final Details: %+v", finalDetails)

	if err != nil {
		t.Fatalf("test failed with error: %v\nLogs:\n%s", err, logBytes.String())
	}

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
