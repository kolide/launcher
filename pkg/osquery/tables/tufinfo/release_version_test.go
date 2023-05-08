package tufinfo

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	tufci "github.com/kolide/launcher/pkg/autoupdate/tuf/ci"
	"github.com/osquery/osquery-go/gen/osquery"

	"github.com/stretchr/testify/require"
)

func TestTufReleaseVersionTable(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UnixNano())

	// Set up some expected results
	expectedResults := make(map[string]string, 0)

	testRootDir := t.TempDir()
	v := randomSemver()
	expectedResults["launcher"] = fmt.Sprintf("launcher/%s/launcher-%s.tar.gz", runtime.GOOS, v)
	expectedResults["osqueryd"] = fmt.Sprintf("osqueryd/%s/osqueryd-%s.tar.gz", runtime.GOOS, v)
	tufci.SeedLocalTufRepo(t, v, testRootDir)

	mockFlags := mocks.NewFlags(t)
	mockFlags.On("RootDirectory").Return(testRootDir)

	// Call table generate func and validate that our data matches what exists in the filesystem
	testTable := TufReleaseVersionTable(mockFlags)
	resp := testTable.Call(context.Background(), osquery.ExtensionPluginRequest{
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
