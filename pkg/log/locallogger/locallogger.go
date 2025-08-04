package locallogger

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"

	// Deduplication configuration
	defaultCacheExpiry   = 5 * time.Minute // How long to remember log entries
	defaultMaxCacheSize  = 1000            // Maximum number of unique log entries to track
	cleanupInterval      = 1 * time.Minute // How often to clean up expired entries
	duplicateLogInterval = 1 * time.Minute // How long to wait before logging a duplicate again
)

// Fields to exclude when creating content hash for deduplication
var excludedHashFields = map[string]bool{
	"ts":     true, // go-kit timestamp
	"time":   true, // slog timestamp
	"caller": true, // go-kit caller info
	"source": true, // slog source info
}

// logEntry tracks information about seen log messages for deduplication
type logEntry struct {
	firstSeen  time.Time
	lastSeen   time.Time
	count      int
	lastLogged time.Time
}

// dedupWriter wraps the actual writer and deduplicates JSON log lines
type dedupWriter struct {
	localLogger  *localLogger
	actualWriter io.Writer
}

// Write implements io.Writer and deduplicates JSON log lines before writing
func (w *dedupWriter) Write(p []byte) (n int, err error) {
	// Handle the case where we receive multiple log lines in one write
	lines := strings.Split(strings.TrimSpace(string(p)), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON to extract fields for deduplication
		var logData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logData); err != nil {
			// If it's not valid JSON, write it directly without deduplication
			if _, writeErr := w.actualWriter.Write([]byte(line + "\n")); writeErr != nil {
				return 0, writeErr
			}
			continue
		}

		// Create a content hash excluding timestamp and caller fields
		hash := w.hashLogData(logData)

		// Check if we should skip this duplicate
		if skip, duplicateCount := w.localLogger.shouldSkipDuplicate(hash); skip {
			continue // Skip this duplicate
		} else if duplicateCount > 1 {
			// Add duplicate count to the log data
			logData["duplicate_count"] = duplicateCount
			// Re-marshal with duplicate count
			if updatedBytes, marshalErr := json.Marshal(logData); marshalErr == nil {
				line = string(updatedBytes)
			}
		}

		// Write the (possibly modified) log line
		if _, writeErr := w.actualWriter.Write([]byte(line + "\n")); writeErr != nil {
			return 0, writeErr
		}
	}

	// Return the number of bytes we were asked to write (even if we skipped some)
	return len(p), nil
}

// hashLogData creates a hash of the log data excluding timestamp and caller fields
func (w *dedupWriter) hashLogData(logData map[string]interface{}) string {
	// Convert map to key-value pairs for the shared hash function
	var keyvals []interface{}
	for key, value := range logData {
		keyvals = append(keyvals, key, value)
	}
	return hashKeyValuePairs(keyvals...)
}

type localLogger struct {
	logger log.Logger
	writer io.Writer
	lj     *lumberjack.Logger

	// Deduplication fields
	dedupCache   map[string]*logEntry
	dedupMutex   sync.RWMutex
	cacheExpiry  time.Duration
	maxCacheSize int
	lastCleanup  time.Time

	// Deduplicating writer for slog and other direct writes
	dedupWriter *dedupWriter
}

func NewKitLogger(logFilePath string) *localLogger {
	// This is meant as an always available debug tool. Thus we hardcode these options
	lj := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    5, // megabytes
		Compress:   true,
		MaxBackups: 8,
	}

	writer := log.NewSyncWriter(lj)

	ll := &localLogger{
		logger: log.With(
			log.NewJSONLogger(writer),
			"ts", log.DefaultTimestampUTC,
			"caller", log.DefaultCaller, ///log.Caller(6),
		),
		lj:     lj, // keep a reference to lumberjack Logger so it can be closed if needed
		writer: writer,

		// Initialize deduplication
		dedupCache:   make(map[string]*logEntry),
		cacheExpiry:  defaultCacheExpiry,
		maxCacheSize: defaultMaxCacheSize,
		lastCleanup:  time.Now(),
	}

	// Create the deduplicating writer that wraps the actual writer
	ll.dedupWriter = &dedupWriter{
		localLogger:  ll,
		actualWriter: writer,
	}

	return ll
}

func (ll *localLogger) Close() error {
	return ll.lj.Close()
}

func (ll *localLogger) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)

	// Check if we should deduplicate this log message
	hash := ll.hashKeyvals(keyvals...)
	skip, duplicateCount := ll.shouldSkipDuplicate(hash)
	if skip {
		return nil
	}

	// If this is a duplicate being logged with count, add the count to keyvals
	if duplicateCount > 1 {
		keyvals = append(keyvals, "duplicate_count", duplicateCount)
	}

	return ll.logger.Log(keyvals...)
}

func (ll *localLogger) Writer() io.Writer {
	return ll.dedupWriter
}

// filterResults filters out the osquery results,
// which just make a lot of noise in our debug logs.
// It's a bit fragile, since it parses keyvals, but
// hopefully that's good enough
func filterResults(keyvals ...interface{}) {
	// Consider switching on `method` as well?
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "results" && len(keyvals) > i+1 {
			str, ok := keyvals[i+1].(string)
			if ok && len(str) > 100 {
				keyvals[i+1] = fmt.Sprintf(truncatedFormatString, str[0:99])
			}
		}
	}
}

// hashKeyvals creates a hash of the key-value pairs for deduplication
// We exclude timestamp and caller fields since they change for identical log content
func (ll *localLogger) hashKeyvals(keyvals ...interface{}) string {
	return hashKeyValuePairs(keyvals...)
}

// hashKeyValuePairs creates a consistent hash from key-value pairs, excluding timestamp fields
func hashKeyValuePairs(keyvals ...interface{}) string {
	hasher := sha256.New()

	// Collect non-excluded key-value pairs
	var pairs []string
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 >= len(keyvals) {
			break
		}

		key := fmt.Sprintf("%v", keyvals[i])

		// Skip excluded fields for content-based deduplication
		if excludedHashFields[key] {
			continue
		}

		value := fmt.Sprintf("%v", keyvals[i+1])
		pairs = append(pairs, key+":"+value)
	}

	// Sort pairs for consistent hashing
	sort.Strings(pairs)

	// Hash the sorted pairs
	for _, pair := range pairs {
		hasher.Write([]byte(pair + ";"))
	}

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// shouldSkipDuplicate checks if this log message is a duplicate and should be skipped
// Returns (skip, duplicateCount) where skip indicates if the message should be skipped
// and duplicateCount is the number of times this message has been seen
func (ll *localLogger) shouldSkipDuplicate(hash string) (bool, int) {
	ll.dedupMutex.Lock()
	defer ll.dedupMutex.Unlock()

	now := time.Now()

	// Perform cleanup if needed
	if now.Sub(ll.lastCleanup) > cleanupInterval {
		ll.cleanupCacheUnsafe(now)
		ll.lastCleanup = now
	}

	entry, exists := ll.dedupCache[hash]
	if !exists {
		// First time seeing this message
		ll.dedupCache[hash] = &logEntry{
			firstSeen:  now,
			lastSeen:   now,
			count:      1,
			lastLogged: now,
		}
		return false, 1 // Don't skip, log it (first occurrence)
	}

	// Update the existing entry
	entry.lastSeen = now
	entry.count++

	// Check if enough time has passed since we last logged this message
	// We use a simple strategy: log the duplicate if it's been more than the interval
	// since we last logged this exact message, and include the count
	if now.Sub(entry.lastLogged) > duplicateLogInterval {
		entry.lastLogged = now
		return false, entry.count // Don't skip, log it with count
	}

	// Skip this duplicate
	return true, entry.count
}

// cleanupCacheUnsafe removes expired entries from the deduplication cache
// Must be called with dedupMutex held
func (ll *localLogger) cleanupCacheUnsafe(now time.Time) {
	// Remove entries older than cacheExpiry
	for hash, entry := range ll.dedupCache {
		if now.Sub(entry.lastSeen) > ll.cacheExpiry {
			delete(ll.dedupCache, hash)
		}
	}

	// If cache is still too large, remove oldest entries
	if len(ll.dedupCache) > ll.maxCacheSize {
		// Create a slice of hashes sorted by lastSeen time
		type hashTime struct {
			hash     string
			lastSeen time.Time
		}

		var entries []hashTime
		for hash, entry := range ll.dedupCache {
			entries = append(entries, hashTime{hash: hash, lastSeen: entry.lastSeen})
		}

		// Sort by lastSeen time (oldest first)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].lastSeen.Before(entries[j].lastSeen)
		})

		// Remove oldest entries until we're under the limit
		toRemove := len(ll.dedupCache) - ll.maxCacheSize
		for i := 0; i < toRemove && i < len(entries); i++ {
			delete(ll.dedupCache, entries[i].hash)
		}
	}
}
