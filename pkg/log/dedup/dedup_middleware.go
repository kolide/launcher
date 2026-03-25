// Package dedup provides a stateful slog handler middleware that suppresses
// bursts of duplicate log records and later emits a summarized record with
// duplicate counts. See dedup_flow.mmd for a visual overview of the runtime behavior.
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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

type nextFunc func(context.Context, slog.Record) error

const (
	DefaultCacheExpiry        = 5 * time.Minute
	DefaultMaxCacheSize       = 2000
	DefaultCleanupInterval    = 1 * time.Minute
	DefaultDuplicateLogWindow = 0

	meterName = "github.com/kolide/launcher/v2/pkg/log/dedup"
)

// Uses the global OTEL meter provider and falls back to noop on error
// to avoid circular dependencies with the ee/observability package.
var (
	dedupSuppressedCounter    metric.Int64Counter
	dedupPassedCounter        metric.Int64Counter
	dedupCacheEntryCountGauge metric.Int64Gauge
	dedupEnabledGauge         metric.Int64Gauge
)

func init() {
	ReinitializeMetrics()
}

// ReinitializeMetrics must be called any time the global OTEL meter provider is
// replaced (see ee/observability/exporter) so metrics bind to the active provider.
func ReinitializeMetrics() {
	m := otel.Meter(meterName)

	var err error

	dedupSuppressedCounter, err = m.Int64Counter("launcher.dedup.suppressed",
		metric.WithDescription("The number of log records suppressed by deduplication"),
		metric.WithUnit("{log}"))
	if err != nil {
		dedupSuppressedCounter = noop.Int64Counter{}
	}

	dedupPassedCounter, err = m.Int64Counter("launcher.dedup.passed",
		metric.WithDescription("The number of log records passed through deduplication"),
		metric.WithUnit("{log}"))
	if err != nil {
		dedupPassedCounter = noop.Int64Counter{}
	}

	dedupCacheEntryCountGauge, err = m.Int64Gauge("launcher.dedup.cache_entry_count",
		metric.WithDescription("Current number of unique log entries in the dedup cache"),
		metric.WithUnit("{entry}"))
	if err != nil {
		dedupCacheEntryCountGauge = noop.Int64Gauge{}
	}

	dedupEnabledGauge, err = m.Int64Gauge("launcher.dedup.enabled",
		metric.WithDescription("Whether log deduplication is enabled"),
		metric.WithUnit("1"))
	if err != nil {
		dedupEnabledGauge = noop.Int64Gauge{}
	}
}

// excludedHashFields are attribute keys excluded from the content hash because
// they vary per emission (timestamps, source location) and would defeat dedup.
var excludedHashFields = map[string]bool{
	"ts":              true,
	"time":            true,
	"caller":          true,
	"source":          true,
	"original.time":   true, // forwarded from desktop/watchdog process (see ee/log)
	"original.source": true, // forwarded from desktop/watchdog process (see ee/log)
}

type Config struct {
	CacheExpiry        time.Duration
	MaxCacheSize       int
	CleanupInterval    time.Duration
	DuplicateLogWindow time.Duration
}

type Option func(*Config)

func WithCacheExpiry(d time.Duration) Option     { return func(c *Config) { c.CacheExpiry = d } }
func WithMaxCacheSize(n int) Option              { return func(c *Config) { c.MaxCacheSize = n } }
func WithCleanupInterval(d time.Duration) Option { return func(c *Config) { c.CleanupInterval = d } }
func WithDuplicateLogWindow(d time.Duration) Option {
	return func(c *Config) { c.DuplicateLogWindow = d }
}

type logEntry struct {
	firstSeen time.Time
	lastSeen  time.Time
	count     int

	// Preserved for summary emission on cleanup
	level   slog.Level
	message string
	attrs   []slog.Attr
	pc      uintptr
	next    nextFunc
}

// Engine is a stateful deduplication engine. It is safe for concurrent use.
type Engine struct {
	cfg Config

	cacheLock      sync.RWMutex
	cache          map[string]*logEntry // maps log hash to corresponding tracked entry
	cleanupRunning atomic.Bool
	started        atomic.Bool

	lifecycleLock                sync.Mutex // protects cancel field for Start/Stop operations
	cancel                       context.CancelFunc
	backGroundCleanUpWorkerGroup sync.WaitGroup

	// Zero or negative disables dedup.
	duplicateLogWindow atomic.Value // of type time.Duration
}

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
	d.duplicateLogWindow.Store(cfg.DuplicateLogWindow)
	return d
}

// handleRecord is the core dedup logic. handlerAttrs are accumulated via
// slog.Logger.With() on the handler chain. They are invisible to slog.Record
// but must be included in the hash so that logs from different handler chains
// (e.g. different "component" values) are not incorrectly deduplicated together.
func (d *Engine) handleRecord(ctx context.Context, record slog.Record, handlerAttrs []slog.Attr, next func(context.Context, slog.Record) error) error {
	if !d.started.Load() || d.getDuplicateLogWindow() <= 0 {
		return next(ctx, record)
	}

	hash := hashRecordWithHandlerAttrs(record, handlerAttrs)

	now := time.Now()

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
			d.cache[hash] = &logEntry{
				firstSeen: now,
				lastSeen:  now,
				count:     1,
				level:     record.Level,
				message:   record.Message,
				attrs:     collectAttrs(record),
				pc:        record.PC,
				next:      nextFunc(next),
			}
			shouldPass = true
			return
		}

		entry.lastSeen = now
		entry.count++
		entry.next = nextFunc(next)
		if now.Sub(entry.firstSeen) >= d.getDuplicateLogWindow() {
			duplicateCount = entry.count
			firstSeen = entry.firstSeen
			lastSeen = entry.lastSeen
			addDuplicateMeta = true
			shouldPass = true
			entry.firstSeen = now
			entry.lastSeen = now
			entry.count = 1
			return
		}

		shouldPass = false
	}()

	if !shouldPass {
		dedupSuppressedCounter.Add(ctx, 1)
		return nil
	}

	dedupPassedCounter.Add(ctx, 1)

	if addDuplicateMeta {
		record.Add("duplicate_count", slog.IntValue(duplicateCount))
		record.Add("first_seen", slog.TimeValue(firstSeen))
		record.Add("last_seen", slog.TimeValue(lastSeen))
	}
	return next(ctx, record)
}

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

func (d *Engine) getDuplicateLogWindow() time.Duration {
	if d == nil {
		return 0
	}
	if v := d.duplicateLogWindow.Load(); v != nil {
		return v.(time.Duration)
	}
	return 0
}

// SetDuplicateLogWindow updates the window atomically. Zero or negative disables dedup.
func (d *Engine) SetDuplicateLogWindow(window time.Duration) {
	if d == nil {
		return
	}
	d.duplicateLogWindow.Store(window)
	d.recordEnabledGauge()
}

func (d *Engine) recordEnabledGauge() {
	var enabled int64
	if d.started.Load() && d.getDuplicateLogWindow() > 0 {
		enabled = 1
	}
	dedupEnabledGauge.Record(context.Background(), enabled)
}

// Start launches the periodic cleanup worker. Subsequent calls are no-ops until Stop.
func (d *Engine) Start(ctx context.Context) {
	if d == nil {
		return
	}
	if d.started.Load() {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)

	d.lifecycleLock.Lock()
	d.cancel = cancel
	d.lifecycleLock.Unlock()

	d.started.Store(true)
	d.recordEnabledGauge()
	d.backGroundCleanUpWorkerGroup.Add(1)
	go d.periodicCleanupLoop(runCtx)
}

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

// performCleanup expires old entries and emits summary records for those with
// duplicate counts. It also evicts the oldest entries when the cache exceeds
// MaxCacheSize. Emission happens outside the lock to avoid re-entrancy deadlocks.
func (d *Engine) performCleanup() {
	now := time.Now()
	if !d.cleanupRunning.CompareAndSwap(false, true) {
		return
	}
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
		next      nextFunc
	}
	var toEmit []expired

	for hash, entry := range d.cache {
		if now.Sub(entry.lastSeen) <= d.cfg.CacheExpiry {
			continue
		}
		if entry.count > 1 {
			toEmit = append(toEmit, expired{
				level:     entry.level,
				message:   entry.message,
				attrs:     append([]slog.Attr(nil), entry.attrs...),
				count:     entry.count,
				pc:        entry.pc,
				firstSeen: entry.firstSeen,
				lastSeen:  entry.lastSeen,
				next:      entry.next,
			})
		}
		delete(d.cache, hash)
	}

	if len(d.cache) > d.cfg.MaxCacheSize {
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
		for i := range removeCount {
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
						next:      entry.next,
					})
				}
				delete(d.cache, items[i].hash)
			}
		}
	}

	cacheSize := int64(len(d.cache))
	d.cacheLock.Unlock()

	dedupCacheEntryCountGauge.Record(context.Background(), cacheSize)

	for _, e := range toEmit {
		// PC preserves the original call site in the summary record
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
		if e.next != nil {
			_ = e.next(context.Background(), rec)
		}
	}
}

// hashRecordWithHandlerAttrs builds a stable content hash from the record and
// any handler-chain attrs/groups. Handler-chain attrs (e.g. "component") are
// added via slog.Logger.With() and invisible to slog.Record; including them
// prevents hash collisions between different handler chains.
func hashRecordWithHandlerAttrs(record slog.Record, handlerAttrs []slog.Attr) string {
	var keyvals []any

	keyvals = append(keyvals, "level", record.Level.String())
	keyvals = append(keyvals, "msg", record.Message)

	for _, attr := range handlerAttrs {
		if !excludedHashFields[attr.Key] {
			keyvals = append(keyvals, attr.Key, attr.Value)
		}
	}

	record.Attrs(func(attr slog.Attr) bool {
		if !excludedHashFields[attr.Key] {
			keyvals = append(keyvals, attr.Key, attr.Value)
		}
		return true
	})

	return hashKeyValuePairs(keyvals...)
}

func collectAttrs(record slog.Record) []slog.Attr {
	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	return attrs
}

// hashKeyValuePairs produces a deterministic SHA-256 hex digest of sorted
// key-value pairs, filtering out excluded fields defensively.
func hashKeyValuePairs(keyvals ...any) string {
	var filtered []any
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

	h := sha256.Sum256(fmt.Appendf(nil, "%v", filtered))
	return fmt.Sprintf("%x", h)
}
