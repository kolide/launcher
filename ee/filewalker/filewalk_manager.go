package filewalker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/gowrapper"
	"github.com/kolide/launcher/v2/ee/observability"
)

// FilewalkNowAction is the control server action forwarded by the actionqueue
// to the filewalk manager to trigger ad hoc filewalks.
const FilewalkNowAction = "filewalk_now"

// FilewalkManager handles configuring and executing filewalkers by the config
// in FilewalkConfigStore.
type FilewalkManager struct {
	// Internals
	cfgStore     types.Iterator
	resultsStore types.GetterSetterDeleterIterator
	slogger      *slog.Logger

	req        chan struct{}
	cancelWalk atomic.Pointer[context.CancelFunc]

	// Handle actor shutdown
	interrupt     chan struct{}
	interruptOnce sync.Once
}

// How often to check filewalks due to be ran.
const walkCheckInterval = 5 * time.Minute

func New(k types.Knapsack, slogger *slog.Logger) *FilewalkManager {
	return &FilewalkManager{
		cfgStore:     k.FilewalkConfigStore(),
		resultsStore: k.FilewalkResultsStore(),
		slogger:      slogger.With("component", "filewalker"),
		req:          make(chan struct{}, 1),
		interrupt:    make(chan struct{}),
	}
}

func (fm *FilewalkManager) Execute() error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	gowrapper.Go(ctx, fm.slogger, func() {
		select {
		case <-fm.interrupt:
			cancel()
		case <-ctx.Done():
		}
	})

	fm.run(ctx)

	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"shut down filewalk manager",
	)
	return nil
}

func (fm *FilewalkManager) Interrupt(_ error) {
	fm.interruptOnce.Do(func() { close(fm.interrupt) })
}

// run is the primary loop for FileWalk manager. It handles kicking off walks,
// incoming requests, and walking when config is dirty.
func (fm *FilewalkManager) run(ctx context.Context) {
	ticker := time.NewTicker(walkCheckInterval)
	defer ticker.Stop()

	var requested bool // full walk requested
	for {
		if cfgs, present, err := fm.pullConfigs(); err != nil {
			fm.slogger.Log(ctx, slog.LevelError,
				"could not pull filewalk configs from store",
				"err", err,
			)
		} else {
			fm.deleteRemovedResults(ctx, present)
			names := due(time.Now(), cfgs, fm.lastWalkTimes(ctx, cfgs))
			if requested {
				names = slices.Collect(maps.Keys(cfgs))
			}
			fm.walkConfigs(ctx, cfgs, names)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			requested = false
		case <-fm.req:
			requested = true
		}
	}
}

func (fm *FilewalkManager) walkConfigs(ctx context.Context, cfgs map[string]filewalkConfig, names []string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fm.cancelWalk.Store(&cancel)

	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		if ctx.Err() != nil {
			return
		}
		cfg, ok := cfgs[name]
		if !ok {
			fm.slogger.Log(ctx, slog.LevelWarn,
				"filewalk request from control server contained unknown filewalk name",
				"filewalk_name", name,
			)
			continue
		}
		newFilewalker(name, cfg, fm.resultsStore, fm.slogger).Filewalk(ctx)
	}
}

// lastWalkTimes returns the map of filewalkConfig names to the last time the config was walked
// successfully.
func (fm *FilewalkManager) lastWalkTimes(ctx context.Context, cfgs map[string]filewalkConfig) map[string]time.Time {
	lastWalks := make(map[string]time.Time, len(cfgs))
	for name := range cfgs {
		raw, err := fm.resultsStore.Get(LastWalkTimeKey(name))
		switch {
		case err != nil:
			fm.slogger.Log(ctx, slog.LevelError,
				"could not get last walk time from store",
				"filewalk_name", name,
				"err", err,
			)
		case len(raw) == 8: // valid values are 64 bits
			lastWalks[name] = time.Unix(int64(binary.NativeEndian.Uint64(raw)), 0)
		case raw != nil:
			fm.slogger.Log(ctx, slog.LevelWarn,
				"invalid last walk time in store",
				"filewalk_name", name,
				"bytes", len(raw),
				"value", raw,
			)
		}
	}
	return lastWalks
}

// returns the filewalkConfig names which are currently due for walking
func due(now time.Time, cfgs map[string]filewalkConfig, lastWalks map[string]time.Time) []string {
	var names []string
	for name, cfg := range cfgs {
		elapsed := now.Sub(lastWalks[name])
		if elapsed < 0 || elapsed >= time.Duration(cfg.WalkInterval) {
			names = append(names, name)
		}
	}
	return names
}

// pullConfigs gets the filewalk configs from the config store.
// First return holds valid configs and the second holds all config names. Error is only returned when neither can be read.
func (fm *FilewalkManager) pullConfigs() (map[string]filewalkConfig, map[string]struct{}, error) {
	cfgs := make(map[string]filewalkConfig)
	present := make(map[string]struct{})
	if err := fm.cfgStore.ForEach(func(k, v []byte) error {
		present[string(k)] = struct{}{}

		var currentCfg filewalkConfig
		if err := json.Unmarshal(v, &currentCfg); err != nil {
			fm.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not unmarshal filewalk config, skipping",
				"filewalker_name", string(k),
				"err", err,
			)
			return nil
		}

		cfgs[string(k)] = currentCfg
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("getting filewalk configs from store: %w", err)
	}

	return cfgs, present, nil
}

// deleteRemovedResults deletes results and last walk times for configs no longer in the config store.
func (fm *FilewalkManager) deleteRemovedResults(ctx context.Context, present map[string]struct{}) {
	lastWalkSuffix := string(LastWalkTimeKey(""))
	isPresent := func(name string) bool {
		_, ok := present[name]
		return ok
	}

	var removed [][]byte
	if err := fm.resultsStore.ForEach(func(k, _ []byte) error {
		key := string(k)
		// Check the whole key first: a config may itself be named "x_last_walk".
		if isPresent(key) {
			return nil
		}
		if name, isLastWalk := strings.CutSuffix(key, lastWalkSuffix); isLastWalk && isPresent(name) {
			return nil
		}
		removed = append(removed, k)
		return nil
	}); err != nil {
		fm.slogger.Log(ctx, slog.LevelError,
			"could not iterate filewalk results",
			"err", err,
		)
		return
	}
	if len(removed) == 0 {
		return
	}

	if err := fm.resultsStore.Delete(removed...); err != nil {
		fm.slogger.Log(ctx, slog.LevelWarn,
			"could not delete results for removed filewalk configs",
			"err", err,
		)
		return
	}
	fm.slogger.Log(ctx, slog.LevelInfo,
		"deleted results for removed filewalk configs",
		"deleted_keys", len(removed),
	)
}

// Ping satisfies the control.subscriber interface -- the manager subscribes to changes to
// the filewalk_config subsystem. Ping cancels any in flight walks.
func (fm *FilewalkManager) Ping() {
	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"processing updated filewalk configs",
	)

	// capture fn to ensure we cancel what was in flight and not us. fine if
	// another req comes in before us. walking twice > walking never.
	cancel := fm.cancelWalk.Load()
	fm.request()
	if cancel != nil {
		(*cancel)()
	}
}

// Do satisfies the actionqueue.actor interface; it allows the control server to request
// immediate filewalk. Never errors. If a request to walk is already queued, another
// is not added.
func (fm *FilewalkManager) Do(data io.Reader) error {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	// request sent down by the control server. names are ignored since walking
	// is not much slower with a few more configs included.
	var req struct {
		FilewalkNames []string `json:"filewalks"`
	}
	if err := json.NewDecoder(data).Decode(&req); err != nil {
		fm.slogger.Log(ctx, slog.LevelWarn,
			"received filewalk request in unexpected format from control server, discarding",
			"err", err,
		)
		// We don't return an error because we don't want the actionqueue to retry this request
		return nil
	}

	fm.slogger.Log(ctx, slog.LevelInfo,
		"received request from control server to perform filewalks now",
		"requested_filewalks", req.FilewalkNames,
	)

	fm.request()
	return nil
}

// request requests a filewalk and returns if another request is queued
func (fm *FilewalkManager) request() {
	select {
	case fm.req <- struct{}{}:
	default:
	}
}
