package osquery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

func TestCollectAndSetEnrollmentDetails_Success(t *testing.T) {
	t.Parallel()

	k := typesmocks.NewKnapsack(t)

	expectedOsquerydPath := "/usr/local/bin/osqueryd"
	k.On("LatestOsquerydPath", mock.Anything).Return(expectedOsquerydPath)
	k.On("SetEnrollmentDetails", mock.AnythingOfType("types.EnrollmentDetails")).Return(nil).Twice()

	ctx := context.Background()
	logger := multislogger.NewNopLogger()

	collectTimeout := 5 * time.Second
	collectRetryInterval := 500 * time.Millisecond

	err := CollectAndSetEnrollmentDetails(ctx, logger, k, collectTimeout, collectRetryInterval)

	require.NoError(t, err)
	k.AssertExpectations(t)

	k.AssertCalled(t, "LatestOsquerydPath", mock.Anything)
	k.AssertNumberOfCalls(t, "SetEnrollmentDetails", 2)
}
