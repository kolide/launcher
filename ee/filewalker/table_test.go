package filewalker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
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
	tableName := "kolide_filewalk_test"
	testFilewalkTable := NewFilewalkTable(tableName, mockFlags, store, multislogger.NewNopLogger())

	// Query the table -- we shouldn't have any results yet, since we haven't performed any filewalks
	response := testFilewalkTable.Call(t.Context(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	require.Equal(t, 0, len(response.Response))

	// Set up a temp directory to filewalk
	testRootDir := t.TempDir()
	expectedFile := filepath.Join(testRootDir, "temp1.txt")
	require.NoError(t, os.WriteFile(expectedFile, []byte("test"), 0755))

	// Set up our filewalker
	cfg := filewalkConfig{
		Name:         tableName,
		WalkInterval: 1 * time.Second,
		RootDir:      testRootDir,
	}
	testFilewalker := newFilewalker(cfg, store, multislogger.NewNopLogger())
	go testFilewalker.Work()
	t.Cleanup(testFilewalker.Stop)

	// Wait for the results to be ready
	time.Sleep(cfg.WalkInterval * 2)

	// Query table again, and check for our expected file
	updatedResponse := testFilewalkTable.Call(t.Context(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})
	require.Equal(t, int32(0), updatedResponse.Status.Code, updatedResponse.Status.Message) // 0 means success
	require.Equal(t, 1, len(updatedResponse.Response))
	require.Equal(t, updatedResponse.Response[0]["path"], expectedFile)
}
