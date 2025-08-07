package locallogger

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

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

	// Test that filterResults truncates long results
	keyvals := []interface{}{
		"level", "info",
		"msg", "test message",
		"results", reallyLongString,
	}

	filterResults(keyvals...)

	// Check that results were truncated
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "results" {
			assert.Equal(t, truncatedLongString, keyvals[i+1])
			break
		}
	}
}

func TestKitLogging(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-kit-logging")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Test basic logging
	err = logger.Log("level", "info", "msg", "test message")
	require.NoError(t, err)

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)
	assert.Len(t, lines, 1, "should have one log entry")

	// Verify JSON structure
	var logEntry map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &logEntry))
	assert.Equal(t, "info", logEntry["level"])
	assert.Equal(t, "test message", logEntry["msg"])
}

// Helper function to filter out empty lines
func filterEmptyLines(lines []string) []string {
	var filtered []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered
}
