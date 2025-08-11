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

// EmittedAttrKey marks a record that is being emitted by the deduper itself
// (e.g., during cleanup) so the middleware can skip deduplication to prevent
// feedback loops.
const EmittedAttrKey = "__dedup_emitted"

// Default configuration values.
const (
	DefaultCacheExpiry        = 5 * time.Minute
	DefaultMaxCacheSize       = 2000
	DefaultCleanupInterval    = 1 * time.Minute
	DefaultDuplicateLogWindow = 1 * time.Minute
)

const duplicateSummaryMsg = "duplicate_summary"

// excludedHashFields are the attribute keys that should not affect the content hash.
var excludedHashFields = map[string]bool{
	"ts":              true, // go-kit timestamp
	"time":            true, // slog timestamp
	"caller":          true, // go-kit caller info
	"source":          true, // slog source info
	"original.time":   true, // slog timestamp forwarded from desktop/watchdog process (see ee/log package)
	"original.source": true, // slog source info forwarded from desktop/watchdog process (see ee/log package)
	EmittedAttrKey:    true, // internal marker
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
	mu          sync.RWMutex
	cache       map[string]*logEntry
	lastCleanup time.Time
	// ensure only one cleanup runs at a time
	cleanupRunning atomic.Bool

	// background cleanup machinery
	ctx    context.Context //nolint:containedctx // Used for background goroutine lifecycle
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// logger used to emit records on cleanup
	logger *slog.Logger
}

// NewEngine creates a new deduplication engine. The provided logger is used to
// emit summary records on expiration; pass the pipeline's logger so records go
// through the same handler chain.
func New(logger *slog.Logger, opts ...Option) *Engine {
	cfg := Config{
		CacheExpiry:        DefaultCacheExpiry,
		MaxCacheSize:       DefaultMaxCacheSize,
		CleanupInterval:    DefaultCleanupInterval,
		DuplicateLogWindow: DefaultDuplicateLogWindow,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Engine{
		cfg:         cfg,
		cache:       make(map[string]*logEntry),
		lastCleanup: time.Now(),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}

	d.startBackgroundCleanup()
	return d
}

// Middleware is an inline slog middleware method bound to this Engine instance.
// It matches slog-multi's inline middleware signature.
func (d *Engine) Middleware(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	// Do not deduplicate debug and lower logs; keep developer visibility.
	if record.Level < slog.LevelInfo {
		return next(ctx, record)
	}
	// If the duplicate window is disabled (<= 0), short-circuit and skip all dedup logic
	if d.cfg.DuplicateLogWindow <= 0 {
		return next(ctx, record)
	}
	// Skip dedup if this is an internally emitted record
	skip := false
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == EmittedAttrKey {
			// Only skip if it is truthy
			if a.Value.Kind() == slog.KindBool && a.Value.Bool() {
				skip = true
			}
		}
		return !skip
	})
	if skip {
		return next(ctx, record)
	}

	// Create a content hash for this record
	hash := d.hashRecord(record)

	// Possibly trigger cleanup in the background (non-blocking)
	d.maybeCleanup()

	// Update dedup state and decide whether to log
	now := time.Now()

	d.mu.Lock()
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
		d.mu.Unlock()
		// First occurrence: let it through unmodified
		return next(ctx, record)
	}

	entry.lastSeen = now
	entry.count++
	if now.Sub(entry.firstSeen) >= d.cfg.DuplicateLogWindow {
		d.mu.Unlock()

		record.Add("duplicate_count", slog.IntValue(entry.count))
		return next(ctx, record)
	}

	// Suppress this duplicate
	d.mu.Unlock()
	return nil
}

// Stop stops the background cleanup goroutine.
func (d *Engine) Stop() {
	d.cancel()
	d.wg.Wait()
}

// SetLogger updates the logger used for emitting records during cleanup. This
// allows wiring the deduper into a pipeline first and then pointing emission to
// that fully constructed pipeline logger.
func (d *Engine) SetLogger(l *slog.Logger) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger = l
}

// startBackgroundCleanup launches the periodic cleanup worker.
func (d *Engine) startBackgroundCleanup() {
	d.wg.Add(1)
	go d.periodicCleanupLoop()
}

// periodicCleanupLoop runs the cleanup ticker until the context is canceled.
func (d *Engine) periodicCleanupLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.runCleanup(time.Now())
		case <-d.ctx.Done():
			return
		}
	}
}

// performCleanup removes expired entries and emits a summary record for each.
func (d *Engine) performCleanup(now time.Time) {
	d.mu.Lock()

	// Remove expired entries
	if now.Sub(d.lastCleanup) < d.cfg.CleanupInterval {
		d.mu.Unlock()
		return
	}
	d.lastCleanup = now

	// Build a list to avoid holding the lock while emitting
	type expired struct {
		level   slog.Level
		message string
		attrs   []slog.Attr
		count   int
		pc      uintptr
	}
	var toEmit []expired

	for hash, entry := range d.cache {
		if now.Sub(entry.lastSeen) > d.cfg.CacheExpiry {
			// Only emit a summary when there were actual duplicates
			if entry.count > 1 {
				toEmit = append(toEmit, expired{
					level:   entry.level,
					message: entry.message,
					attrs:   append([]slog.Attr(nil), entry.attrs...),
					count:   entry.count,
					pc:      entry.pc,
				})
			}
			delete(d.cache, hash)
		}
	}

	// Enforce max cache size by removing oldest without emission
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
			delete(d.cache, items[i].hash)
		}
	}

	d.mu.Unlock()

	// Emit outside the lock to avoid re-entrancy deadlocks
	for _, e := range toEmit {
		// Build a record that preserves the original call site via PC
		rec := slog.NewRecord(time.Now(), e.level, duplicateSummaryMsg, 0)
		rec.PC = e.pc
		for _, a := range e.attrs {
			rec.AddAttrs(a)
		}
		rec.AddAttrs(
			slog.Int("duplicate_count", e.count),
			slog.Bool(EmittedAttrKey, true),
			slog.String("original_msg", e.message),
		)
		// Emit using the provided logger's handler so it traverses the pipeline
		if d.logger != nil {
			_ = d.logger.Handler().Handle(context.Background(), rec)
		}
	}
}

// maybeCleanup triggers cleanup based on time since last cleanup.
func (d *Engine) maybeCleanup() {
	d.mu.RLock()
	last := d.lastCleanup
	d.mu.RUnlock()
	if time.Since(last) >= d.cfg.CleanupInterval {
		// Best-effort cleanup run in the background; the periodic ticker will also handle it
		go d.runCleanup(time.Now())
	}
}

// runCleanup executes performCleanup but ensures only one cleanup is running concurrently.
func (d *Engine) runCleanup(now time.Time) {
	if !d.cleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer d.cleanupRunning.Store(false)
	d.performCleanup(now)
}

// hashRecord creates a hash of the log record content, excluding time and source information.
func (d *Engine) hashRecord(record slog.Record) string {
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
