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
	"strings"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
)

type (
	// windowsUpdatesCacher queries for fresh Windows updates data every `cacheInterval`,
	// and stores it in the `cacheStore`.
	windowsUpdatesCacher struct {
		cacheStore    types.GetterSetter
		cacheInterval time.Duration
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

func NewWindowsUpdatesCacher(cacheStore types.GetterSetter, cacheInterval time.Duration, slogger *slog.Logger) *windowsUpdatesCacher {
	return &windowsUpdatesCacher{
		cacheStore:    cacheStore,
		cacheInterval: cacheInterval,
		slogger:       slogger.With("component", "windows_updates_cacher"),
		interrupt:     make(chan struct{}, 1), // provide a buffer for the channel so that Interrupt can send to it and return immediately
	}
}

func (w *windowsUpdatesCacher) Execute() (err error) {
	cacheTicker := time.NewTicker(w.cacheInterval)
	defer cacheTicker.Stop()

	for {
		// Since this query happens in the background and will not block auth, we can use
		// a much longer timeout than we use for our tables.
		var ctx context.Context
		ctx, w.queryCancel = context.WithTimeout(context.Background(), 10*time.Minute)
		if err := w.queryAndStoreData(ctx); err != nil {
			w.slogger.Log(ctx, slog.LevelError,
				"error caching windows update data",
				"err", err,
			)
			// Increment our counter tracking query failures/timeouts
			observability.WindowsUpdatesQueryFailureCounter.Add(ctx, 1)
		} else {
			w.slogger.Log(ctx, slog.LevelDebug,
				"successfully cached windows updates data",
			)
		}
		w.queryCancel()

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
	if w.interrupted.Load() {
		return
	}
	w.interrupted.Store(true)

	// If we have a long-running query going right now, cancel it so that it doesn't prevent
	// shutdown.
	if w.queryCancel != nil {
		w.queryCancel()
	}

	w.interrupt <- struct{}{}
}

// queryAndStoreData will query the Windows Update Agent API via the `launcher query-windowsupdates`
// subcommand. If desired, a timeout can be created for the `ctx` arg; it will be respected by the
// exec to `launcher query-windowsupdates`.
func (w *windowsUpdatesCacher) queryAndStoreData(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	launcherPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting path to launcher: %w", err)
	}
	if !strings.HasSuffix(launcherPath, "launcher.exe") {
		return errors.New("cannot run generate for non-launcher executable (is this running in a test context?)")
	}

	queryTime := time.Now()
	res, err := callQueryWindowsUpdatesSubcommand(ctx, launcherPath, defaultLocale, UpdatesTable)
	if err != nil {
		return fmt.Errorf("running query windows updates subcommand: %w", err)
	}

	rawResultsToStore, err := json.Marshal(&cachedQueryResults{
		QueryTime: queryTime,
		Results:   res,
	})
	if err != nil {
		return fmt.Errorf("marshalling results to store: %w", err)
	}

	if err := w.cacheStore.Set([]byte(defaultLocale), rawResultsToStore); err != nil {
		return fmt.Errorf("setting query results in store: %w", err)
	}

	return nil
}
