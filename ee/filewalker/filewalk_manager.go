package filewalker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/gowrapper"
	"github.com/kolide/launcher/v2/ee/observability"
)

// FilewalkNowAction is the control server action forwarded by the actionqueue
// to the filewalk manager to trigger ad hoc filewalks.
const FilewalkNowAction = "filewalk_now"

// controlServerFilewalkRequest is the request sent down by the control server
// to trigger ad hoc filewalks. If the list of filewalks is empty, the manager
// will trigger filewalks for all of its filewalkers.
type controlServerFilewalkRequest struct {
	FilewalkNames []string `json:"filewalks"`
}

// FilewalkManager handles configuring and executing filewalkers by the config
// in FilewalkConfigStore.
type FilewalkManager struct {
	// Internals
	k            types.Knapsack
	cfgStore     types.Iterator
	resultsStore types.GetterSetterDeleter
	slogger      *slog.Logger

	dirty *atomic.Bool
	doReq chan []string

	// Handle actor shutdown
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

// How often to check filewalks due to be ran.
const walkCheckInterval = 5 * time.Minute

func New(k types.Knapsack, slogger *slog.Logger) *FilewalkManager {
	return &FilewalkManager{
		k:            k,
		cfgStore:     k.FilewalkConfigStore(),
		resultsStore: k.FilewalkResultsStore(),
		slogger:      slogger.With("component", "filewalker"),
		dirty:        &atomic.Bool{},
		doReq:        make(chan []string, 1),
		interrupt:    make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		interrupted:  &atomic.Bool{},
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
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if fm.interrupted.Swap(true) {
		return
	}

	fm.interrupt <- struct{}{}
}

// run is the primary loop for FileWalk manager. It handles kicking off walks,
// incoming requests, and walking when config is dirty.
func (fm *FilewalkManager) run(ctx context.Context) {
	ticker := time.NewTicker(walkCheckInterval)
	defer ticker.Stop()

	var requested []string
	for {
		extraNames := requested
		requested = nil
		if cfgs, err := fm.pullConfigs(); err != nil {
			fm.slogger.Log(ctx, slog.LevelError,
				"could not pull filewalk configs from store",
				"err", err,
			)
		} else {
			names := due(time.Now(), cfgs, fm.lastWalkTimes(cfgs))
			if fm.dirty.Swap(false) {
				names = slices.Collect(maps.Keys(cfgs))
			} else {
				names = append(names, extraNames...)
			}
			fm.walkConfigs(ctx, cfgs, names)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case requested = <-fm.doReq:
			if len(requested) == 0 {
				fm.dirty.Store(true)
			}
		}
	}
}

func (fm *FilewalkManager) walkConfigs(ctx context.Context, cfgs map[string]filewalkConfig, names []string) {
	toWalk := make(map[string]filewalkConfig, len(names))
	for _, name := range names {
		if cfg, ok := cfgs[name]; ok {
			toWalk[name] = cfg
		} else {
			fm.slogger.Log(ctx, slog.LevelWarn,
				"filewalk request from control server contained unknown filewalk name",
				"filewalk_name", name,
			)
		}
	}
	if len(toWalk) == 0 {
		return
	}

	spec := resolve(ctx, fm.slogger, toWalk, runtime.GOOS)
	newFilewalker(spec, fm.resultsStore, fm.slogger).Filewalk(ctx)
}

// lastWalkTimes returns the map of filewalkConfig name to the last time the config was walked
// successfully.
func (fm *FilewalkManager) lastWalkTimes(cfgs map[string]filewalkConfig) map[string]time.Time {
	lastWalks := make(map[string]time.Time, len(cfgs))
	for name := range cfgs {
		if lastWalk, ok := fm.lastWalkTime(name); ok {
			lastWalks[name] = lastWalk
		}
	}
	return lastWalks
}

// lastWalkTime returns the last time the filewalkConfig by name was walked successfully.
// If the name is unknown, the bool is false.
func (fm *FilewalkManager) lastWalkTime(name string) (time.Time, bool) {
	raw, err := fm.resultsStore.Get(LastWalkTimeKey(name))
	if err != nil || len(raw) != 8 {
		return time.Time{}, false
	}
	return time.Unix(int64(binary.NativeEndian.Uint64(raw)), 0), true
}

// returns the filewalkConfig names which are currently due for walking
func due(now time.Time, cfgs map[string]filewalkConfig, lastWalks map[string]time.Time) []string {
	var names []string
	for _, name := range slices.Sorted(maps.Keys(cfgs)) {
		lastWalk, walked := lastWalks[name]
		if !walked || lastWalk.After(now) || !now.Before(lastWalk.Add(time.Duration(cfgs[name].WalkInterval))) {
			names = append(names, name)
		}
	}
	return names
}

// pullConfigs gets the filewalk configs from the config store.
func (fm *FilewalkManager) pullConfigs() (map[string]filewalkConfig, error) {
	cfgs := make(map[string]filewalkConfig, 0)
	if err := fm.cfgStore.ForEach(func(k, v []byte) error {
		var currentCfg filewalkConfig
		if err := json.Unmarshal(v, &currentCfg); err != nil {
			return fmt.Errorf("unmarshalling filewalk config for %s: %w", string(k), err)
		}

		cfgs[string(k)] = currentCfg
		return nil
	}); err != nil {
		return nil, fmt.Errorf("getting filewalk configs from store: %w", err)
	}

	return cfgs, nil
}

// Ping satisfies the control.subscriber interface -- the manager subscribes to changes to
// the filewalk_config subsystem.
func (fm *FilewalkManager) Ping() {
	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"processing updated filewalk configs",
	)

	// TODO: removed configs keep their stored results until cleanup lands.
	fm.dirty.Store(true)
}

// Do satisfies the actionqueue.actor interface; it allows the control server to send
// requests down to filewalk immediately.
func (fm *FilewalkManager) Do(data io.Reader) error {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	var req controlServerFilewalkRequest
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

	select {
	case fm.doReq <- req.FilewalkNames:
		return nil
	default:
		return errors.New("filewalk request already in flight")
	}
}
