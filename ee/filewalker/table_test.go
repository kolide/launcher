package filewalker

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFilewalkTable(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	// Set up table
	walkName := "kolide_filewalk_test"
	testFilewalkTable := NewFilewalkTable(mockFlags, store, multislogger.NewNopLogger())

	// Query the table -- we shouldn't have any results yet, since we haven't performed any filewalks
	response := testFilewalkTable.Call(t.Context(), ci.BuildRequestWithSingleEqualConstraint("walk_name", walkName))
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	require.Equal(t, 0, len(response.Response))

	// Set up a temp directory to filewalk
	testRootDir := t.TempDir()
	expectedFile := filepath.Join(testRootDir, "temp1.txt")
	require.NoError(t, os.WriteFile(expectedFile, []byte("test"), 0755))

	// Set up our filewalker
	cfg := filewalkConfig{
		WalkInterval: duration(1 * time.Second),
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &[]string{testRootDir},
			FileNameRegex: nil,
		},
	}
	testFilewalker := newFilewalker(walkName, cfg, store, multislogger.NewNopLogger())
	startTime := time.Now().Unix()
	go testFilewalker.Work()
	t.Cleanup(testFilewalker.Stop)

	// Wait for the results to be ready
	time.Sleep(time.Duration(cfg.WalkInterval * 2))

	// Query table again, and check for our expected file
	updatedResponse := testFilewalkTable.Call(t.Context(), ci.BuildRequestWithSingleEqualConstraint("walk_name", walkName))
	require.Equal(t, int32(0), updatedResponse.Status.Code, updatedResponse.Status.Message) // 0 means success
	require.Equal(t, 2, len(updatedResponse.Response))                                      // One file, one directory => 2 total rows
	require.Equal(t, walkName, updatedResponse.Response[0]["walk_name"])
	require.Equal(t, walkName, updatedResponse.Response[1]["walk_name"])
	require.Equal(t, testRootDir, updatedResponse.Response[0]["path"]) // We can't always guarantee ordering with filewalk results, but we can for this one
	require.Equal(t, expectedFile, updatedResponse.Response[1]["path"])
	lastWalkTimestamp, err := strconv.Atoi(updatedResponse.Response[0]["last_walk_timestamp"])
	require.NoError(t, err)
	require.Less(t, startTime, int64(lastWalkTimestamp))
}
