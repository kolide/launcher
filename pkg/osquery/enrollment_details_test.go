package osquery

import (
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

	err1 := getOsqEnrollDetails(t.Context(), filepath.Join("some", "fake", "path", "to", "osqueryd"), &details)
	require.Error(t, err1, "expected error when path does not exist")

	err2 := getOsqEnrollDetails(t.Context(), t.TempDir(), &details)
	require.Error(t, err2, "expected error when path is directory")
}

func Test_getEnrollDetails_executionError(t *testing.T) {
	t.Parallel()

	var details service.EnrollmentDetails

	currentExecutable, err := os.Executable()
	require.NoError(t, err, "could not get current executable for test")

	// We expect getEnrollDetails to fail when called against an executable that is not osquery
	err = getOsqEnrollDetails(t.Context(), currentExecutable, &details)
	require.Error(t, err, "should not have been able to get enroll details with non-osqueryd executable")
}

func TestCollectAndSetEnrollmentDetails_EmptyPath(t *testing.T) {
	t.Parallel()
	k := typesmocks.NewKnapsack(t)
	ctx := t.Context()
	slogger := multislogger.NewNopLogger()

	k.On("LatestOsquerydPath", mock.Anything).Return("")
	k.On("SetEnrollmentDetails", mock.AnythingOfType("types.EnrollmentDetails")).Return(nil).Once()

	CollectAndSetEnrollmentDetails(ctx, slogger, k, 1*time.Second, 100*time.Millisecond)

	k.AssertNumberOfCalls(t, "SetEnrollmentDetails", 1)
}

func TestCollectAndSetEnrollmentDetails_Success(t *testing.T) {
	t.Parallel()

	k := typesmocks.NewKnapsack(t)

	expectedOsquerydPath := "/some/fake/path/to/osquerd"
	k.On("LatestOsquerydPath", mock.Anything).Return(expectedOsquerydPath)
	k.On("SetEnrollmentDetails", mock.AnythingOfType("types.EnrollmentDetails")).Twice()

	ctx := t.Context()
	logger := multislogger.NewNopLogger()

	collectTimeout := 5 * time.Second
	collectRetryInterval := 500 * time.Millisecond

	CollectAndSetEnrollmentDetails(ctx, logger, k, collectTimeout, collectRetryInterval)

	k.AssertExpectations(t)

	k.AssertCalled(t, "LatestOsquerydPath", mock.Anything)
	k.AssertNumberOfCalls(t, "SetEnrollmentDetails", 2)
}
