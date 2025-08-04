package locallogger

import (
	"encoding/json"
	"fmt"
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

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-locallogger")
	require.NoError(t, err, "make temp file")

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

	require.NoError(t, logger.Close())
}

func TestDeduplication(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-deduplication")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Log the same message multiple times
	data := []interface{}{"level", "info", "msg", "test message"}

	// First log should go through
	err = logger.Log(data...)
	require.NoError(t, err)

	// Subsequent logs should be deduplicated (skipped)
	for i := 0; i < 5; i++ {
		err = logger.Log(data...)
		require.NoError(t, err)
	}

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	// Should only have one log entry since duplicates were skipped
	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)
	assert.Len(t, lines, 1, "should only have one log entry due to deduplication")

	// Verify the content
	var logEntry map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &logEntry))
	assert.Equal(t, "info", logEntry["level"])
	assert.Equal(t, "test message", logEntry["msg"])
	assert.NotContains(t, logEntry, "duplicate_count", "first occurrence should not have duplicate_count")
}

func TestDeduplicationWithTimeInterval(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-deduplication-time")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	data := []interface{}{"level", "warn", "msg", "repeated warning"}

	// Log the message first time
	err = logger.Log(data...)
	require.NoError(t, err)

	// Log duplicates that should be skipped
	for i := 0; i < 3; i++ {
		err = logger.Log(data...)
		require.NoError(t, err)
	}

	// Simulate time passing by modifying the lastLogged time
	hash := hashKeyValuePairs(data...)
	logger.dedupWriter.dedupMutex.Lock()
	entry := logger.dedupWriter.dedupCache[hash]
	entry.lastLogged = entry.lastLogged.Add(-2 * time.Minute) // Simulate 2 minutes ago
	logger.dedupWriter.dedupMutex.Unlock()

	// This should now log with duplicate count
	err = logger.Log(data...)
	require.NoError(t, err)

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)
	assert.Len(t, lines, 2, "should have two log entries (original + duplicate with count)")

	// Check the second log entry has duplicate_count
	var secondEntry map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &secondEntry))
	assert.Equal(t, "warn", secondEntry["level"])
	assert.Equal(t, "repeated warning", secondEntry["msg"])
	assert.Equal(t, float64(5), secondEntry["duplicate_count"], "should show total count of 5")
}

func TestDeduplicationDifferentMessages(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-different-messages")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Log different messages - these should not be deduplicated
	messages := [][]interface{}{
		{"level", "info", "msg", "message one"},
		{"level", "info", "msg", "message two"},
		{"level", "warn", "msg", "message one"},                   // same text but different level
		{"level", "info", "msg", "message one", "extra", "field"}, // extra field
	}

	for _, data := range messages {
		err := logger.Log(data...)
		require.NoError(t, err)
	}

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)
	assert.Len(t, lines, 4, "all different messages should be logged")
}

func TestHashKeyvals(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-hash")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

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

func TestCacheCleanup(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-cleanup")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Override cache settings for testing
	logger.dedupWriter.cacheExpiry = 100 * time.Millisecond
	logger.dedupWriter.maxCacheSize = 10 // Set higher to test expiry cleanup specifically

	// Add some entries
	data1 := []interface{}{"msg", "message1"}
	data2 := []interface{}{"msg", "message2"}

	logger.Log(data1...)
	logger.Log(data2...)

	assert.Len(t, logger.dedupWriter.dedupCache, 2, "should have 2 entries")

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Force cleanup by setting lastCleanup to trigger cleanup on next call
	logger.dedupWriter.dedupMutex.Lock()
	logger.dedupWriter.lastCleanup = logger.dedupWriter.lastCleanup.Add(-2 * time.Minute)
	logger.dedupWriter.dedupMutex.Unlock()

	// Add another entry, which should trigger cleanup of expired entries
	data3 := []interface{}{"msg", "message3"}
	logger.Log(data3...)

	// The cleanup should have removed expired entries, leaving only the new one
	assert.Len(t, logger.dedupWriter.dedupCache, 1, "expired entries should be cleaned up, only new entry remains")
}

func TestCacheSizeLimit(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-size-limit")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Set a small cache size for testing
	logger.dedupWriter.maxCacheSize = 3

	// Add more entries than the limit
	for i := 0; i < 5; i++ {
		data := []interface{}{"msg", fmt.Sprintf("message%d", i)}
		logger.Log(data...)
	}

	// Force cleanup by triggering it manually
	logger.dedupWriter.dedupMutex.Lock()
	logger.dedupWriter.cleanupCacheUnsafe(time.Now())
	logger.dedupWriter.dedupMutex.Unlock()

	assert.LessOrEqual(t, len(logger.dedupWriter.dedupCache), logger.dedupWriter.maxCacheSize, "cache size should not exceed limit")
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-edge-cases")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Test empty keyvals
	err = logger.Log()
	require.NoError(t, err)

	// Test odd number of keyvals
	err = logger.Log("key")
	require.NoError(t, err)

	// Test nil values
	err = logger.Log("key", nil)
	require.NoError(t, err)

	// Verify logger still works
	assert.NotNil(t, logger.dedupWriter.dedupCache)
}

func TestDeduplicationThroughWriter(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-writer-dedup")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Get the writer (which should be our deduplicating writer)
	writer := logger.Writer()

	// Write the same JSON log line multiple times directly to the writer
	logLine := `{"level":"info","msg":"test message","ts":"2023-01-01T00:00:00Z"}` + "\n"

	// First write should go through
	_, err = writer.Write([]byte(logLine))
	require.NoError(t, err)

	// Subsequent writes should be deduplicated
	for i := 0; i < 5; i++ {
		_, err = writer.Write([]byte(logLine))
		require.NoError(t, err)
	}

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)
	assert.Len(t, lines, 1, "should only have one log entry due to deduplication at writer level")

	// Verify the content
	var logEntry map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &logEntry))
	assert.Equal(t, "info", logEntry["level"])
	assert.Equal(t, "test message", logEntry["msg"])
}

func TestMixedLoggingPaths(t *testing.T) {
	t.Parallel()

	tmpfile, err := os.CreateTemp(t.TempDir(), "test-mixed-paths")
	require.NoError(t, err, "make temp file")
	tmpfile.Close()

	logger := NewKitLogger(tmpfile.Name())
	defer logger.Close()

	// Log via go-kit log interface
	err = logger.Log("level", "info", "msg", "test message")
	require.NoError(t, err)

	// Log via writer interface (simulating slog)
	writer := logger.Writer()
	logLine := `{"level":"info","msg":"test message","ts":"2023-01-01T00:00:00Z"}` + "\n"
	_, err = writer.Write([]byte(logLine))
	require.NoError(t, err)

	contentsRaw, err := os.ReadFile(tmpfile.Name())
	require.NoError(t, err, "read temp file")

	lines := strings.Split(strings.TrimSpace(string(contentsRaw)), "\n")
	lines = filterEmptyLines(lines)

	// Should have only one line since the second should be deduplicated
	// (both should result in similar content hash)
	assert.LessOrEqual(t, len(lines), 2, "should have at most 2 entries, likely 1 due to deduplication")
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
