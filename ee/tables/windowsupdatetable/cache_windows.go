//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/observability"
)

type (
	// windowsUpdatesCacher queries for fresh Windows updates data every `cacheInterval`,
	// and stores it in the `cacheStore`.
	windowsUpdatesCacher struct {
		flags         types.Flags
		cacheStore    types.GetterSetter
		cacheInterval time.Duration
		cacheLock     *sync.Mutex
		queryCancel   context.CancelFunc
		slogger       *slog.Logger
		interrupt     chan struct{}
		interrupted   atomic.Bool
	}

	// cachedQueryResults stores the results of querying the Windows Update Agent API;
	// it includes a timestamp so we know how fresh this data is.
	cachedQueryResults struct {
		QueryTime time.Time     `json:"timestamp"`
		Results   *QueryResults `json:"results"`
	}
)

func NewWindowsUpdatesCacher(flags types.Flags, cacheStore types.GetterSetter, cacheInterval time.Duration, slogger *slog.Logger) *windowsUpdatesCacher {
	w := &windowsUpdatesCacher{
		flags:         flags,
		cacheStore:    cacheStore,
		cacheInterval: cacheInterval,
		cacheLock:     &sync.Mutex{},
		slogger:       slogger.With("component", "windows_updates_cacher"),
		interrupt:     make(chan struct{}, 1), // provide a buffer for the channel so that Interrupt can send to it and return immediately
	}
	flags.RegisterChangeObserver(w, keys.InModernStandby)

	return w
}

func (w *windowsUpdatesCacher) Execute() (err error) {
	cacheTicker := time.NewTicker(w.cacheInterval)
	defer cacheTicker.Stop()

	for {
		if w.flags.InModernStandby() {
			// We subscribe to changes in `keys.InModernStandby`, so we'll instead make a cache attempt
			// as soon as we exit modern standby.
			w.slogger.Log(context.TODO(), slog.LevelDebug,
				"skipping caching while in modern standby",
			)
		} else {
			if err := w.queryAndStoreData(context.TODO()); err != nil {
				w.slogger.Log(context.TODO(), slog.LevelError,
					"error caching windows update data",
					"err", err,
				)
				// Increment our counter tracking query failures/timeouts
				observability.WindowsUpdatesQueryFailureCounter.Add(context.TODO(), 1)
			} else {
				w.slogger.Log(context.TODO(), slog.LevelDebug,
					"successfully cached windows updates data",
				)
			}
		}

		select {
		case <-cacheTicker.C:
			continue
		case <-w.interrupt:
			return nil
		}
	}
}

func (w *windowsUpdatesCacher) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if w.interrupted.Swap(true) {
		return
	}

	// If we have a long-running query going right now, cancel it so that it doesn't prevent
	// shutdown.
	if w.queryCancel != nil {
		w.queryCancel()
	}

	w.interrupt <- struct{}{}
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface. The cacher subscribes to changes to
// InModernStandby.
func (w *windowsUpdatesCacher) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// If we've exited modern standby, kick off a query-and-cache attempt. We do this in a goroutine
	// to avoid blocking notifying other observers of changed flags.
	if slices.Contains(flagKeys, keys.InModernStandby) && !w.flags.InModernStandby() {
		gowrapper.Go(ctx, w.slogger, func() {
			// Wait slightly, to make sure we've successfully exited modern standby -- sometimes
			// modern standby status flaps.
			time.Sleep(15 * time.Second)
			if w.flags.InModernStandby() {
				return
			}
			if err := w.queryAndStoreData(ctx); err != nil {
				observability.SetError(span, err)
				w.slogger.Log(ctx, slog.LevelError,
					"error caching windows update data after exiting modern standby",
					"err", err,
				)
				// Increment our counter tracking query failures/timeouts
				observability.WindowsUpdatesQueryFailureCounter.Add(context.TODO(), 1)
				return
			}

			w.slogger.Log(ctx, slog.LevelDebug,
				"successfully cached windows updates data after exiting modern standby",
			)
		})
	}
}

// queryAndStoreData will query the Windows Update Agent API via the `launcher query-windowsupdates`
// subcommand. If desired, a timeout can be created for the `ctx` arg; it will be respected by the
// exec to `launcher query-windowsupdates`.
func (w *windowsUpdatesCacher) queryAndStoreData(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	w.cacheLock.Lock()
	defer w.cacheLock.Unlock()
	span.AddEvent("cache_lock_acquired")

	// Make sure that the rungroup was not interrupted while waiting for cache lock
	if w.interrupted.Load() {
		err := errors.New("interrupted while waiting for cache lock, will not proceed with querying")
		observability.SetError(span, err)
		return err
	}

	// Since this query happens in the background and will not block auth, we can use
	// a much longer timeout than we use for our tables. We set queryCancel on windowsUpdateCacher
	// so that we can `Interrupt` ongoing query attempts on launcher shutdown if needed.
	ctx, w.queryCancel = context.WithTimeout(ctx, 20*time.Minute)
	defer w.queryCancel()

	launcherPath, err := os.Executable()
	if err != nil {
		err = fmt.Errorf("getting path to launcher: %w", err)
		observability.SetError(span, err)
		return err
	}
	if !strings.HasSuffix(launcherPath, "launcher.exe") {
		err = fmt.Errorf("cannot run generate for non-launcher executable %s (is this running in a test context?)", launcherPath)
		observability.SetError(span, err)
		return err
	}

	queryTime := time.Now()
	res, err := callQueryWindowsUpdatesSubcommand(ctx, launcherPath, defaultLocale, UpdatesTable)
	if err != nil {
		err = fmt.Errorf("running query windows updates subcommand: %w", err)
		observability.SetError(span, err)
		return err
	}

	rawResultsToStore, err := json.Marshal(&cachedQueryResults{
		QueryTime: queryTime,
		Results:   res,
	})
	if err != nil {
		err = fmt.Errorf("marshalling results to store: %w", err)
		observability.SetError(span, err)
		return err
	}

	if err := w.cacheStore.Set([]byte(defaultLocale), rawResultsToStore); err != nil {
		err = fmt.Errorf("setting query results in store: %w", err)
		observability.SetError(span, err)
		return err
	}

	return nil
}
