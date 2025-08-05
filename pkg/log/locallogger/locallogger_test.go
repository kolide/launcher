package locallogger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

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

func TestDedupHandler(t *testing.T) {
	t.Parallel()

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create base JSON handler
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		AddSource: false, // Disable source to make testing easier
		Level:     slog.LevelDebug,
	})

	// Create dedup handler
	dedupHandler := NewDedupHandler(jsonHandler)
	defer dedupHandler.Close()

	// Create logger with dedup handler
	logger := slog.New(dedupHandler)

	// Test basic deduplication
	logger.InfoContext(context.Background(), "test message", "key", "value")
	logger.InfoContext(context.Background(), "test message", "key", "value") // Should be skipped
	logger.InfoContext(context.Background(), "test message", "key", "value") // Should be skipped

	// Now manipulate the cache to simulate time passing
	// We need to create the same record that would be created by logger.Info
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("key", "value"))
	hash := dedupHandler.hashRecord(record)

	dedupHandler.dedupMutex.Lock()
	entry := dedupHandler.dedupCache[hash]
	if entry != nil {
		entry.lastLogged = entry.lastLogged.Add(-2 * time.Minute) // Simulate old timestamp
	}
	dedupHandler.dedupMutex.Unlock()

	// This should now log with duplicate count immediately (no sleep needed)
	logger.InfoContext(context.Background(), "test message", "key", "value") // Should be logged with duplicate_count

	// Get output
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Filter out empty lines
	var actualLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			actualLines = append(actualLines, line)
		}
	}

	// Should have exactly 2 lines: first occurrence and duplicate with count
	require.Len(t, actualLines, 2, "Expected exactly 2 log lines after deduplication")

	// Verify the second line contains duplicate_count
	require.Contains(t, actualLines[1], "duplicate_count", "Second line should contain duplicate count")

	// Verify the duplicate count is 4 (we sent 4 identical messages)
	require.Contains(t, actualLines[1], `"duplicate_count":4`, "Should show correct duplicate count")
}

func TestDedupHandlerWithDifferentMessages(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
	})

	dedupHandler := NewDedupHandler(jsonHandler)
	defer dedupHandler.Close()

	logger := slog.New(dedupHandler)

	// Different messages should not be deduplicated
	logger.InfoContext(context.Background(), "message one", "key", "value1")
	logger.InfoContext(context.Background(), "message two", "key", "value2")
	logger.InfoContext(context.Background(), "message one", "key", "value1") // Should be skipped
	logger.InfoContext(context.Background(), "message two", "key", "value2") // Should be skipped

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var actualLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			actualLines = append(actualLines, line)
		}
	}

	// Should have exactly 2 lines (one for each unique message)
	require.Len(t, actualLines, 2, "Expected exactly 2 log lines for different messages")

	// Neither should contain duplicate_count since they're different
	require.NotContains(t, actualLines[0], "duplicate_count", "First line should not contain duplicate count")
	require.NotContains(t, actualLines[1], "duplicate_count", "Second line should not contain duplicate count")
}

func TestSlogHandlerDedup(t *testing.T) {
	t.Parallel()

	// Test that the new slog handler approach works correctly
	logger := NewKitLogger("")

	// Get the slog handler
	handler := logger.SlogHandler()
	require.NotNil(t, handler, "SlogHandler should return a valid handler")

	// Test that it's a dedup handler
	_, ok := handler.(*DedupHandler)
	require.True(t, ok, "SlogHandler should return a DedupHandler")
}

func TestHashKeyValuePairs(t *testing.T) {
	t.Parallel()

	// Test that timestamp and caller fields are excluded from hash
	data1 := []interface{}{"level", "info", "msg", "test", "ts", "2023-01-01T00:00:00Z", "caller", "file.go:123"}
	data2 := []interface{}{"level", "info", "msg", "test", "ts", "2023-01-01T00:01:00Z", "caller", "file.go:456"}
	data3 := []interface{}{"level", "info", "msg", "different"}

	hash1 := hashKeyValuePairs(data1...)
	hash2 := hashKeyValuePairs(data2...)
	hash3 := hashKeyValuePairs(data3...)

	assert.Equal(t, hash1, hash2, "hashes should be equal despite different ts and caller")
	assert.NotEqual(t, hash1, hash3, "hashes should be different for different content")
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
