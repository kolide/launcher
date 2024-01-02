package checkups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/stretchr/testify/require"
)

func TestRun_Tuf(t *testing.T) {
	t.Parallel()

	tempRootDir := t.TempDir()
	testReleaseVersion := "2.3.4"

	// Set up a test TUF server to serve legitimate metadata
	tufServerUrl, _ := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up mock knapsack
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Autoupdate").Return(true)
	mockKnapsack.On("KolideHosted").Return(true)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("RootDirectory").Return(tempRootDir)
	mockKnapsack.On("UpdateDirectory").Return("")

	testTufCheckup := &tufCheckup{k: mockKnapsack}

	output := &bytes.Buffer{}

	require.NoError(t, testTufCheckup.Run(context.TODO(), output), "did not expect error running checkup")

	// Validate status
	require.Equal(t, Passing, testTufCheckup.status, "expected passing status")

	// Validate summary -- just confirm we set something
	require.Greater(t, len(testTufCheckup.summary), 0, "expected summary to be set")

	// Validate data
	expectedDataKey := fmt.Sprintf("%s/repository/targets.json", tufServerUrl)
	require.Contains(t, testTufCheckup.data, expectedDataKey, "missing data")
	launcherTarget, ok := testTufCheckup.data[expectedDataKey].(string)
	require.True(t, ok, "unexpected type for data key")
	require.True(t, strings.HasSuffix(launcherTarget, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)))

	// Validate tuf.json has expected data
	var d map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &d))
	require.Contains(t, d, "remote_metadata_version")
	require.Contains(t, d, "local_metadata_version")
	require.Contains(t, d, "launcher_versions_in_library")
	require.Contains(t, d, "osqueryd_versions_in_library")
	require.Contains(t, d, "selected_versions")
}
