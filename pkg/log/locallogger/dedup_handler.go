package locallogger

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/gowrapper"
)

// DedupHandler implements slog.Handler and provides content-based deduplication
type DedupHandler struct {
	handler slog.Handler

	// Deduplication state
	dedupCache   map[string]*logEntry
	dedupMutex   *sync.RWMutex // Use pointer to avoid copying in WithAttrs/WithGroup
	cacheExpiry  time.Duration
	maxCacheSize int
	lastCleanup  time.Time

	// Background cleanup
	cleanupTrigger chan struct{}
	cleanupDone    chan struct{}
	ctx            context.Context //nolint:containedctx // Used for background goroutine lifecycle
	cancel         context.CancelFunc
	slogger        *slog.Logger
}

// NewDedupHandler creates a new deduplication handler that wraps another slog.Handler
func NewDedupHandler(handler slog.Handler) *DedupHandler {
	ctx, cancel := context.WithCancel(context.Background())

	dh := &DedupHandler{
		handler:        handler,
		dedupCache:     make(map[string]*logEntry),
		dedupMutex:     &sync.RWMutex{}, // Create pointer to mutex
		cacheExpiry:    defaultCacheExpiry,
		maxCacheSize:   defaultMaxCacheSize,
		lastCleanup:    time.Now(),
		cleanupTrigger: make(chan struct{}, 1),
		cleanupDone:    make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
		slogger:        slog.Default(),
	}

	// Start background cleanup goroutine using gowrapper
	dh.startBackgroundCleanup()

	return dh
}

// Enabled implements slog.Handler.Enabled
func (dh *DedupHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return dh.handler.Enabled(ctx, level)
}

// Handle implements slog.Handler.Handle
func (dh *DedupHandler) Handle(ctx context.Context, record slog.Record) error {
	// Create a hash of the log record content (excluding time and source)
	hash := dh.hashRecord(record)

	// Check if we should skip this duplicate
	if skip, duplicateCount := dh.shouldSkipDuplicate(hash); skip {
		return nil // Skip this duplicate
	} else if duplicateCount > 1 {
		// Add duplicate count to the record
		record.AddAttrs(slog.Int("duplicate_count", duplicateCount))
	}

	// Pass the (possibly modified) record to the underlying handler
	return dh.handler.Handle(ctx, record)
}

// WithAttrs implements slog.Handler.WithAttrs
func (dh *DedupHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &DedupHandler{
		handler:        dh.handler.WithAttrs(attrs),
		dedupCache:     dh.dedupCache,
		dedupMutex:     dh.dedupMutex, // Share the same mutex pointer
		cacheExpiry:    dh.cacheExpiry,
		maxCacheSize:   dh.maxCacheSize,
		lastCleanup:    dh.lastCleanup,
		cleanupTrigger: dh.cleanupTrigger,
		cleanupDone:    dh.cleanupDone,
		ctx:            dh.ctx,
		cancel:         dh.cancel,
		slogger:        dh.slogger,
	}
}

// WithGroup implements slog.Handler.WithGroup
func (dh *DedupHandler) WithGroup(name string) slog.Handler {
	return &DedupHandler{
		handler:        dh.handler.WithGroup(name),
		dedupCache:     dh.dedupCache,
		dedupMutex:     dh.dedupMutex, // Share the same mutex pointer
		cacheExpiry:    dh.cacheExpiry,
		maxCacheSize:   dh.maxCacheSize,
		lastCleanup:    dh.lastCleanup,
		cleanupTrigger: dh.cleanupTrigger,
		cleanupDone:    dh.cleanupDone,
		ctx:            dh.ctx,
		cancel:         dh.cancel,
		slogger:        dh.slogger,
	}
}

// Close shuts down the background cleanup goroutine
func (dh *DedupHandler) Close() {
	dh.cancel()
	<-dh.cleanupDone
}

// hashRecord creates a hash of the log record content, excluding time and source information
func (dh *DedupHandler) hashRecord(record slog.Record) string {
	// Convert record to key-value pairs for hashing
	var keyvals []interface{}

	// Add level and message
	keyvals = append(keyvals, "level", record.Level.String())
	keyvals = append(keyvals, "msg", record.Message)

	// Add all attributes except excluded ones
	record.Attrs(func(attr slog.Attr) bool {
		key := attr.Key
		if !excludedHashFields[key] {
			keyvals = append(keyvals, key, attr.Value)
		}
		return true
	})

	return hashKeyValuePairs(keyvals...)
}

// shouldSkipDuplicate checks if this log message is a duplicate and should be skipped
func (dh *DedupHandler) shouldSkipDuplicate(hash string) (bool, int) {
	now := time.Now()

	// Always attempt cleanup - the cleanup function decides if it's needed
	dh.cleanupCache(now) // This call is non-blocking and triggers async cleanup

	dh.dedupMutex.Lock()
	defer dh.dedupMutex.Unlock()

	entry, exists := dh.dedupCache[hash]
	if !exists {
		// First time seeing this message
		dh.dedupCache[hash] = &logEntry{
			firstSeen:  now,
			lastSeen:   now,
			count:      1,
			lastLogged: now,
		}
		return false, 1
	}

	// Update existing entry
	entry.lastSeen = now
	entry.count++

	// Check if we should log this duplicate
	if now.Sub(entry.lastLogged) >= duplicateLogInterval {
		entry.lastLogged = now
		return false, entry.count // Log with duplicate count
	}

	return true, entry.count // Skip this duplicate
}

// triggerCleanupIfNeeded triggers background cleanup if needed
func (dh *DedupHandler) triggerCleanupIfNeeded(now time.Time) {
	select {
	case dh.cleanupTrigger <- struct{}{}:
		// Cleanup triggered successfully
	default:
		// Cleanup already in progress, skip
	}
}

// startBackgroundCleanup starts the background cleanup goroutine using gowrapper
func (dh *DedupHandler) startBackgroundCleanup() {
	gowrapper.Go(dh.ctx, dh.slogger, func() {
		dh.backgroundCleanup()
	})
}

// backgroundCleanup runs in a separate goroutine and handles cache cleanup
func (dh *DedupHandler) backgroundCleanup() {
	defer close(dh.cleanupDone)

	for {
		select {
		case _, ok := <-dh.cleanupTrigger:
			if !ok {
				// Channel was closed, time to shutdown
				dh.slogger.Log(dh.ctx, slog.LevelDebug,
					"background cleanup goroutine shutting down (trigger channel closed)",
				)
				return
			}
			dh.performCleanup()
		case <-dh.ctx.Done():
			dh.slogger.Log(dh.ctx, slog.LevelDebug,
				"background cleanup goroutine shutting down (context cancelled)",
			)
			return
		}
	}
}

// performCleanup removes expired entries from the deduplication cache
func (dh *DedupHandler) performCleanup() {
	now := time.Now()

	dh.dedupMutex.Lock()
	defer dh.dedupMutex.Unlock()

	// Check if cleanup is actually needed
	if now.Sub(dh.lastCleanup) <= cleanupInterval {
		return
	}

	// Update cleanup timestamp
	dh.lastCleanup = now

	// Remove expired entries
	expiredCount := 0
	for hash, entry := range dh.dedupCache {
		if now.Sub(entry.lastSeen) > dh.cacheExpiry {
			delete(dh.dedupCache, hash)
			expiredCount++
		}
	}

	// If cache is still too large, remove oldest entries
	if len(dh.dedupCache) > dh.maxCacheSize {
		// Create slice of entries with their last seen times
		type hashTime struct {
			hash     string
			lastSeen time.Time
		}
		var entries []hashTime
		for hash, entry := range dh.dedupCache {
			entries = append(entries, hashTime{hash, entry.lastSeen})
		}

		// Sort by last seen time (oldest first)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].lastSeen.Before(entries[j].lastSeen)
		})

		// Remove oldest entries to get down to maxCacheSize
		toRemove := len(entries) - dh.maxCacheSize
		for i := 0; i < toRemove; i++ {
			delete(dh.dedupCache, entries[i].hash)
		}

		dh.slogger.Log(dh.ctx, slog.LevelDebug,
			"deduplication cache cleanup completed",
			"expired_entries", expiredCount,
			"removed_oldest", toRemove,
			"remaining_entries", len(dh.dedupCache),
		)
	} else if expiredCount > 0 {
		dh.slogger.Log(dh.ctx, slog.LevelDebug,
			"deduplication cache cleanup completed",
			"expired_entries", expiredCount,
			"remaining_entries", len(dh.dedupCache),
		)
	}
}

// cleanupCache is a wrapper that calls performCleanup with proper locking
func (dh *DedupHandler) cleanupCache(now time.Time) {
	dh.triggerCleanupIfNeeded(now)
}
