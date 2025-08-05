package locallogger

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/go-kit/kit/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"

	// Deduplication configuration
	defaultCacheExpiry   = 5 * time.Minute // How long to remember log entries
	defaultMaxCacheSize  = 2000            // Maximum number of unique log entries to track
	cleanupInterval      = 1 * time.Minute // How often to clean up expired entries
	duplicateLogInterval = 1 * time.Minute // How long to wait before logging a duplicate again
)

// Fields to exclude when creating content hash for deduplication
var excludedHashFields = map[string]bool{
	"ts":              true, // go-kit timestamp
	"time":            true, // slog timestamp
	"caller":          true, // go-kit caller info
	"source":          true, // slog source info
	"original.time":   true,
	"original.source": true,
}

// logEntry tracks information about seen log messages for deduplication
type logEntry struct {
	firstSeen  time.Time
	lastSeen   time.Time
	count      int
	lastLogged time.Time
}

// localLogger provides local file logging with deduplication support
type localLogger struct {
	logger log.Logger
	writer io.Writer
	lj     *lumberjack.Logger

	// Deduplication handler for slog-based logging
	dedupHandler *DedupHandler
}

// NewKitLogger creates a new local logger with deduplication support
func NewKitLogger(logFilePath string) *localLogger {
	// Create lumberjack logger for file rotation
	lj := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    100, // megabytes
		MaxBackups: 3,
		MaxAge:     28,   // days
		Compress:   true, // compress rotated files
	}

	// Create base JSON handler for slog
	jsonHandler := slog.NewJSONHandler(lj, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})

	// Wrap with deduplication handler
	dedupHandler := NewDedupHandler(jsonHandler)

	// Create go-kit logger that writes to the same file
	writer := log.NewSyncWriter(lj)
	logger := log.NewJSONLogger(writer)

	return &localLogger{
		logger:       logger,
		writer:       writer,
		lj:           lj, // keep a reference to lumberjack Logger so it can be closed if needed
		dedupHandler: dedupHandler,
	}
}

// Close closes the logger and its underlying resources
func (ll *localLogger) Close() error {
	// Close dedup handler first
	ll.dedupHandler.Close()
	// Then close lumberjack
	return ll.lj.Close()
}

// Log implements go-kit logging interface
func (ll *localLogger) Log(keyvals ...interface{}) error {
	return ll.logger.Log(keyvals...)
}

// Writer returns the underlying io.Writer for direct access
func (ll *localLogger) Writer() io.Writer {
	return ll.writer
}

// SlogHandler returns the slog.Handler with deduplication support
func (ll *localLogger) SlogHandler() slog.Handler {
	return ll.dedupHandler
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

// hashKeyValuePairs creates a hash of key-value pairs for deduplication
// This function is shared between different deduplication approaches
func hashKeyValuePairs(keyvals ...interface{}) string {
	// Filter out excluded fields
	var filtered []interface{}
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			key := fmt.Sprintf("%v", keyvals[i])
			if !excludedHashFields[key] {
				filtered = append(filtered, keyvals[i], keyvals[i+1])
			}
		}
	}

	// Sort for consistent hashing
	sort.Slice(filtered, func(i, j int) bool {
		if i%2 == 0 && j%2 == 0 {
			return fmt.Sprintf("%v", filtered[i]) < fmt.Sprintf("%v", filtered[j])
		}
		return i < j
	})

	// Create hash
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", filtered)))
	return fmt.Sprintf("%x", h)
}
