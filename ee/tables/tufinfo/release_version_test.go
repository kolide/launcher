package tufinfo

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tuf"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/gen/osquery"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTufReleaseVersionTable(t *testing.T) {
	t.Parallel()

	// Set up some expected results
	expectedResults := make(map[string]string, 0)

	testRootDir := t.TempDir()
	v := randomSemver()
	expectedResults["launcher"] = fmt.Sprintf("launcher/%s/%s/launcher-%s.tar.gz", runtime.GOOS, tuf.PlatformArch(), v)
	expectedResults["osqueryd"] = fmt.Sprintf("osqueryd/%s/%s/osqueryd-%s.tar.gz", runtime.GOOS, tuf.PlatformArch(), v)
	tufci.SeedLocalTufRepo(t, v, testRootDir)

	mockFlags := mocks.NewFlags(t)
	mockFlags.On("RootDirectory").Return(testRootDir)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	// Call table generate func and validate that our data matches what exists in the filesystem
	testTable := TufReleaseVersionTable(multislogger.NewNopLogger(), mockFlags)
	resp := testTable.Call(t.Context(), osquery.ExtensionPluginRequest{
		"action":  "generate",
		"context": "{}",
	})

	// Require success
	require.Equal(t, int32(0), resp.Status.Code, fmt.Sprintf("unexpected failure generating table: %s", resp.Status.Message))

	// Require results
	require.Greater(t, len(resp.Response), 0, "expected results but did not receive any")

	for _, row := range resp.Response {
		expectedTarget, ok := expectedResults[row["binary"]]
		require.True(t, ok, "found unexpected row: %v", row)
		require.Equal(t, expectedTarget, row["target"], "target mismatch")
	}
}

func randomSemver() string {
	major := rand.Intn(100) - 1
	minor := rand.Intn(100) - 1
	patch := rand.Intn(100) - 1

	// Only sometimes include hash
	includeHash := rand.Int()
	if includeHash%2 == 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}

	randUuid := uuid.New().String()

	return fmt.Sprintf("%d.%d.%d-%s", major, minor, patch, randUuid[0:8])
}
