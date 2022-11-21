package locallogger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/stringutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	reallyLongString    = stringutil.RandomString(200)
	truncatedLongString = fmt.Sprintf(truncatedFormatString, reallyLongString[0:99])
)

func TestFilterResults(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		"one", "two",
		"results", reallyLongString,
	}

	filterResults(data...)
	assert.Len(t, data, 4)
	assert.Equal(t, data[0], "one")
	assert.Equal(t, data[1], "two")
	assert.Equal(t, data[2], "results")
	assert.Len(t, data[3], 110)
	assert.Contains(t, data[3], "[TRUNCATED]")
	assert.Equal(t, data[3], truncatedLongString)
}

func TestKitLogging(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		"one", "two",
		"results", reallyLongString,
	}

	expected := map[string]string{
		"one":     "two",
		"results": truncatedLongString,
	}
	//	expectedJson, err := json.Marshal(expected)
	//require.NoError(t, err, "json marshal expected")

	tmpfile, err := ioutil.TempFile("", "test-locallogger")
	require.NoError(t, err, "make temp file")
	defer os.Remove(tmpfile.Name())

	// we only need a file path, not the file handle
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())

	logger.Log(data...)

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	var contents map[string]string
	require.NoError(t, json.Unmarshal(contentsRaw, &contents), "unmarshal json")

	// can't compare the whole thing, since we have extra values from timestamp and caller
	for k, v := range expected {
		assert.Equal(t, v, contents[k])
	}

}

func TestCleanUpRenamedDebugLogs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create two files that should be cleaned up and one that should not
	legacyDebugLogPath := filepath.Join(tempDir, "debug.log")
	legacyDebugLogRotatedPath := filepath.Join(tempDir, "debug-2022-11-18T18-35-48.858.log.gz")
	newDebugLogPath := filepath.Join(tempDir, "debug.json")
	for _, f := range []string{legacyDebugLogPath, legacyDebugLogRotatedPath, newDebugLogPath} {
		fh, err := os.Create(f)
		require.NoError(t, err, "could not create log file for test")
		fh.Close()
	}

	// Call cleanup
	CleanUpRenamedDebugLogs(tempDir, log.NewJSONLogger(os.Stderr))

	// Validate that we only cleaned up the files we meant to
	_, err := os.Stat(legacyDebugLogPath)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(legacyDebugLogRotatedPath)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(newDebugLogPath)
	require.NoError(t, err)

	// Call cleanup again -- should be a no-op
	CleanUpRenamedDebugLogs(tempDir, log.NewJSONLogger(os.Stderr))

	_, err = os.Stat(newDebugLogPath)
	require.NoError(t, err)
}
