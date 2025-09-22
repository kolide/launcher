// Package dedup provides a stateful slog middleware that suppresses bursts of
// duplicate log records and later emits a summarized record with duplicate
// counts. It computes a content hash of each record (excluding timestamps and
// source metadata) to identify duplicates within a configurable time window.
//
// High-level flow:
//   - Incoming slog.Record enters the middleware chain
//   - If DuplicateLogWindow <= 0, pass-through (dedup disabled)
//   - Compute a hash of the record's stable content
//   - Update or create the cache entry:
//   - First time: store entry and pass the record through unmodified
//   - Subsequent times within the window: increment count and suppress
//   - Once the window elapses for that entry: pass the record with
//     duplicate_count/first_seen/last_seen attributes
//   - In the background, a periodic cleanup removes expired entries and, for
//     those that had duplicates, emits a summarized record preserving the
//     original message, attributes, and call site (PC).
//
// Mermaid overview of the runtime behavior:
// ```mermaid
// flowchart TD
//     A["Incoming slog.Record"] --> B{"DuplicateLogWindow ≤ 0?"}
//     B -- Yes --> N["Pass‑through (dedup disabled)"]
//     B -- No  --> D["Compute stable content hash"]

//     D --> F{"Entry exists in cache?"}

//     F -- No  --> G["Create new entry<br/> count = 1; firstSeen = lastSeen = now"]
//     G --> N

//     F -- Yes --> H["entry.count++<br/>entry.lastSeen = now"]
//     H --> I{"Window elapsed (now - firstSeen ≥ DuplicateLogWindow)?"}

//     I -- No  --> S["Suppress (return nil)"]
//     I -- Yes --> J["Pass record with duplicate_count/first_seen/last_seen attrs"]
//     J --> N

//     %% Background maintenance
//     subgraph "Background cleanup (periodic)"
//         T["Every CleanupInterval"] --> U["performCleanup()"]
//         U --> V{"Entry expired (now - lastSeen > CacheExpiry)?"}

//         V -- Yes --> X["If count > 1:<br/> emit summary record<br/>(original msg/attrs/PC + duplicate_count)"]
//         V -- No  --> W{"Cache size > MaxCacheSize?"}

//	    W -- Yes --> Y["Evict oldest; if count > 1 emit summary"]
//	end
//
// ```
package dedup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// nextFunc represents the downstream slog-multi inline middleware function
// used to forward a record to the next handler in the chain.
type nextFunc func(context.Context, slog.Record) error

// Default configuration values.
const (
	DefaultCacheExpiry        = 5 * time.Minute
	DefaultMaxCacheSize       = 2000
	DefaultCleanupInterval    = 1 * time.Minute
	DefaultDuplicateLogWindow = 0
)

// excludedHashFields are the attribute keys that should not affect the content hash.
var excludedHashFields = map[string]bool{
	"ts":              true, // go-kit timestamp
	"time":            true, // slog timestamp
	"caller":          true, // go-kit caller info
	"source":          true, // slog source info
	"original.time":   true, // slog timestamp forwarded from desktop/watchdog process (see ee/log package)
	"original.source": true, // slog source info forwarded from desktop/watchdog process (see ee/log package)
}

// Config controls the behavior of the dedup middleware.
type Config struct {
	CacheExpiry        time.Duration
	MaxCacheSize       int
	CleanupInterval    time.Duration
	DuplicateLogWindow time.Duration
}

// Option configures the deduper.
type Option func(*Config)

// WithCacheExpiry overrides the default cache expiry.
func WithCacheExpiry(d time.Duration) Option { return func(c *Config) { c.CacheExpiry = d } }

// WithMaxCacheSize overrides the default maximum cache size.
func WithMaxCacheSize(n int) Option { return func(c *Config) { c.MaxCacheSize = n } }

// WithCleanupInterval overrides the cleanup interval.
func WithCleanupInterval(d time.Duration) Option { return func(c *Config) { c.CleanupInterval = d } }

// WithDuplicateLogWindow overrides the window to re-log duplicates.
func WithDuplicateLogWindow(d time.Duration) Option {
	return func(c *Config) { c.DuplicateLogWindow = d }
}

// logEntry tracks information about seen log messages for deduplication.
type logEntry struct {
	firstSeen time.Time
	lastSeen  time.Time
	count     int

	// For emission on cleanup
	level   slog.Level
	message string
	attrs   []slog.Attr
	pc      uintptr
}

// Engine is a stateful deduplication engine. It is safe for concurrent use.
type Engine struct {
	// configuration
	cfg Config

	// runtime state
	cacheLock sync.RWMutex
	cache     map[string]*logEntry // maps log hash to corresponding tracked entry
	// ensure only one cleanup runs at a time
	cleanupRunning atomic.Bool
	// started indicates whether Start(ctx) has been called and the engine is active
	started atomic.Bool

	// background cleanup machinery
	lifecycleLock                sync.Mutex // protects cancel field for Start/Stop operations
	cancel                       context.CancelFunc
	backGroundCleanUpWorkerGroup sync.WaitGroup

	// lastNext holds the most recently seen downstream middleware function used to
	// forward records during background cleanup emissions. Stored via atomic.Value
	// to allow lock-free reads from the cleanup goroutine.
	lastNext atomic.Value // of type nextFunc

	// duplicateLogWindow holds the current duplicate log window duration for thread-safe access.
	// When zero or negative, deduplication is disabled.
	duplicateLogWindow atomic.Value // of type time.Duration
}

// New creates a new deduplication engine. The engine keeps a reference to the
// most recently observed downstream 'next' function (from Middleware) and uses
// it to emit summary records during background cleanup so they flow through the
// same handler chain.
func New(opts ...Option) *Engine {
	cfg := Config{
		CacheExpiry:        DefaultCacheExpiry,
		MaxCacheSize:       DefaultMaxCacheSize,
		CleanupInterval:    DefaultCleanupInterval,
		DuplicateLogWindow: DefaultDuplicateLogWindow,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	d := &Engine{
		cfg:   cfg,
		cache: make(map[string]*logEntry),
	}
	// Initialize the atomic duplicate log window with the config value
	d.duplicateLogWindow.Store(cfg.DuplicateLogWindow)
	return d
}

// Middleware is an inline slog middleware method bound to this Engine instance.
// It matches slog-multi's inline middleware signature.
func (d *Engine) Middleware(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	// If the engine hasn't been started or duplicate log window is disabled,
	// act as a no-op middleware.
	if !d.started.Load() || d.getDuplicateLogWindow() <= 0 {
		return next(ctx, record)
	}

	// Remember the latest downstream 'next' so background cleanup can emit
	// summary records through the same pipeline.
	d.lastNext.Store(nextFunc(next))

	// Create a content hash for this record
	hash := hashRecord(record)

	// Update dedup state and decide whether to log
	now := time.Now()

	// Compute action under lock, then release before calling next
	shouldPass := false
	addDuplicateMeta := false
	var duplicateCount int
	var firstSeen time.Time
	var lastSeen time.Time

	func() {
		d.cacheLock.Lock()
		defer d.cacheLock.Unlock()

		entry, exists := d.cache[hash]
		if !exists {
			attrs := collectAttrs(record)
			d.cache[hash] = &logEntry{
				firstSeen: now,
				lastSeen:  now,
				count:     1,
				level:     record.Level,
				message:   record.Message,
				attrs:     attrs,
				pc:        record.PC,
			}
			// First occurrence passes through unchanged
			shouldPass = true
			return
		}

		entry.lastSeen = now
		entry.count++
		// Window for tracking this particular log has elapsed -- relog with duplicate metadata
		if now.Sub(entry.firstSeen) >= d.getDuplicateLogWindow() {
			duplicateCount = entry.count
			firstSeen = entry.firstSeen
			lastSeen = entry.lastSeen
			addDuplicateMeta = true
			shouldPass = true
			return
		}

		// Otherwise, suppress this duplicate
		shouldPass = false
	}()

	if !shouldPass {
		return nil
	}
	if addDuplicateMeta {
		record.Add("duplicate_count", slog.IntValue(duplicateCount))
		record.Add("first_seen", slog.TimeValue(firstSeen))
		record.Add("last_seen", slog.TimeValue(lastSeen))
	}
	return next(ctx, record)
}

// Stop stops the background cleanup goroutine.
func (d *Engine) Stop() {
	if d == nil {
		return
	}
	d.lifecycleLock.Lock()
	cancel := d.cancel
	d.lifecycleLock.Unlock()

	if cancel != nil {
		cancel()
	}
	d.backGroundCleanUpWorkerGroup.Wait()
	d.started.Store(false)
}

// getDuplicateLogWindow returns the current duplicate log window duration atomically.
func (d *Engine) getDuplicateLogWindow() time.Duration {
	if d == nil {
		return 0
	}
	if v := d.duplicateLogWindow.Load(); v != nil {
		return v.(time.Duration)
	}
	return 0
}

// SetDuplicateLogWindow updates the duplicate log window duration atomically.
// When set to zero or negative, deduplication is effectively disabled.
func (d *Engine) SetDuplicateLogWindow(window time.Duration) {
	if d == nil {
		return
	}
	d.duplicateLogWindow.Store(window)
}

// startBackgroundCleanup launches the periodic cleanup worker.
// Start launches the periodic cleanup worker bound to the provided context.
// Subsequent calls are no-ops until Stop is called.
func (d *Engine) Start(ctx context.Context) {
	if d == nil {
		return
	}
	// If already started, do nothing.
	if d.started.Load() {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)

	d.lifecycleLock.Lock()
	d.cancel = cancel
	d.lifecycleLock.Unlock()

	d.started.Store(true)
	d.backGroundCleanUpWorkerGroup.Add(1)
	go d.periodicCleanupLoop(runCtx)
}

// periodicCleanupLoop runs the cleanup ticker until the context is canceled.
func (d *Engine) periodicCleanupLoop(ctx context.Context) {
	defer d.backGroundCleanUpWorkerGroup.Done()
	ticker := time.NewTicker(d.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.performCleanup()
		case <-ctx.Done():
			return
		}
	}
}

// performCleanup removes expired entries and emits a summary record for each.
func (d *Engine) performCleanup() {
	now := time.Now()
	// Ensure only one cleanup runs at a time
	if !d.cleanupRunning.CompareAndSwap(false, true) {
		return
	}

	// Build a list to avoid holding the lock while emitting
	defer d.cleanupRunning.Store(false)
	d.cacheLock.Lock()

	type expired struct {
		level     slog.Level
		message   string
		attrs     []slog.Attr
		count     int
		pc        uintptr
		firstSeen time.Time
		lastSeen  time.Time
	}
	var toEmit []expired

	for hash, entry := range d.cache {
		if now.Sub(entry.lastSeen) <= d.cfg.CacheExpiry {
			continue
		}
		// Only emit a summary when there were actual duplicates
		if entry.count > 1 {
			toEmit = append(toEmit, expired{
				level:     entry.level,
				message:   entry.message,
				attrs:     append([]slog.Attr(nil), entry.attrs...),
				count:     entry.count,
				pc:        entry.pc,
				firstSeen: entry.firstSeen,
				lastSeen:  entry.lastSeen,
			})
		}
		delete(d.cache, hash)
	}

	// Enforce max cache size by removing oldest; emit summaries for duplicates being evicted
	if len(d.cache) > d.cfg.MaxCacheSize {
		// Collect entries with lastSeen
		type hashTime struct {
			hash     string
			lastSeen time.Time
		}
		items := make([]hashTime, 0, len(d.cache))
		for h, e := range d.cache {
			items = append(items, hashTime{hash: h, lastSeen: e.lastSeen})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].lastSeen.Before(items[j].lastSeen) })
		removeCount := len(d.cache) - d.cfg.MaxCacheSize
		for i := 0; i < removeCount; i++ {
			if entry, ok := d.cache[items[i].hash]; ok {
				if entry.count > 1 {
					toEmit = append(toEmit, expired{
						level:     entry.level,
						message:   entry.message,
						attrs:     append([]slog.Attr(nil), entry.attrs...),
						count:     entry.count,
						pc:        entry.pc,
						firstSeen: entry.firstSeen,
						lastSeen:  entry.lastSeen,
					})
				}
				delete(d.cache, items[i].hash)
			}
		}
	}

	d.cacheLock.Unlock()

	// Emit outside the lock to avoid re-entrancy deadlocks
	for _, e := range toEmit {
		// Build a record that preserves the original call site via PC and uses the original message
		rec := slog.NewRecord(time.Now(), e.level, e.message, 0)
		rec.PC = e.pc
		for _, a := range e.attrs {
			rec.AddAttrs(a)
		}
		rec.AddAttrs(
			slog.Int("duplicate_count", e.count),
			slog.String("original_msg", e.message),
			slog.Time("first_seen", e.firstSeen),
			slog.Time("last_seen", e.lastSeen),
		)
		// Emit using the most recently observed downstream 'next' so it
		// traverses the same pipeline. Best-effort if available.
		if v := d.lastNext.Load(); v != nil {
			if n, ok := v.(nextFunc); ok && n != nil {
				_ = n(context.Background(), rec)
			}
		}
	}
}

// hashRecord creates a hash of the log record content, excluding time and source information.
func hashRecord(record slog.Record) string {
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

// collectAttrs copies the attributes from a record into a slice, preserving order.
func collectAttrs(record slog.Record) []slog.Attr {
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	return attrs
}

// hashKeyValuePairs creates a hash of key-value pairs for deduplication.
func hashKeyValuePairs(keyvals ...interface{}) string {
	// Filter out excluded fields (defensive; hashRecord already filters)
	var filtered []interface{}
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			key := fmt.Sprintf("%v", keyvals[i])
			if !excludedHashFields[key] {
				filtered = append(filtered, keyvals[i], keyvals[i+1])
			}
		}
	}

	// Sort by key for stable hashing
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
