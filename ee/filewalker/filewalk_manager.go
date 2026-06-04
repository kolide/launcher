package filewalker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

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

// FilewalkManager creates and starts all configured filewalkers, and handles
// updates to the filewalker configs.
type FilewalkManager struct {
	filewalkers     map[string]*filewalker
	filewalkersLock *sync.Mutex

	// Internals
	k        types.Knapsack
	cfgStore types.Iterator
	slogger  *slog.Logger

	// Handle actor shutdown
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func New(k types.Knapsack, slogger *slog.Logger) *FilewalkManager {
	return &FilewalkManager{
		filewalkers:     make(map[string]*filewalker),
		filewalkersLock: &sync.Mutex{},
		k:               k,
		cfgStore:        k.FilewalkConfigStore(),
		slogger:         slogger.With("component", "filewalker"),
		interrupt:       make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		interrupted:     &atomic.Bool{},
	}
}

func (fm *FilewalkManager) Execute() error {
	// Init filewalkers
	cfgs, err := fm.pullConfigs()
	if err != nil {
		fm.slogger.Log(context.TODO(), slog.LevelError,
			"failed to pull filewalk configs, will not be able to initialize filewalkers until subsystem data is updated",
			"err", err,
		)
	}
	fm.filewalkersLock.Lock()
	for filewalkerName, cfg := range cfgs {
		fm.filewalkers[filewalkerName] = newFilewalker(filewalkerName, cfg, fm.k.FilewalkResultsStore(), fm.slogger)
		gowrapper.Go(context.TODO(), fm.slogger, fm.filewalkers[filewalkerName].Work)
	}
	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"started all filewalkers",
		"filewalker_count", len(fm.filewalkers),
	)
	fm.filewalkersLock.Unlock()

	// Wait for shutdown, then clean up all filewalkers
	<-fm.interrupt
	fm.filewalkersLock.Lock()
	defer fm.filewalkersLock.Unlock()
	for _, fw := range fm.filewalkers {
		fw.Stop()
	}
	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"shut down all filewalkers",
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
	fm.filewalkersLock.Lock()
	defer fm.filewalkersLock.Unlock()

	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"processing updated filewalk configs",
	)

	// Pull the updated config from the store.
	cfgs, err := fm.pullConfigs()
	if err != nil {
		fm.slogger.Log(context.TODO(), slog.LevelError,
			"could not pull updated configs from store",
			"err", err,
		)
		return
	}

	// Check for filewalkers to add or update
	for filewalkerName, cfg := range cfgs {
		if fw, alreadyExists := fm.filewalkers[filewalkerName]; alreadyExists {
			fw.UpdateConfig(cfg)
			// Kick off a new filewalk in the background, to populate results with the updated config
			gowrapper.Go(context.TODO(), fm.slogger, func() { fw.Filewalk(context.TODO()) })
		} else {
			// Add the new filewalker
			fm.filewalkers[filewalkerName] = newFilewalker(filewalkerName, cfg, fm.k.FilewalkResultsStore(), fm.slogger)
			gowrapper.Go(context.TODO(), fm.slogger, fm.filewalkers[filewalkerName].Work)
		}
	}

	// Now, check to see if we need to shut down and delete any filewalkers
	for filewalkerName, fw := range fm.filewalkers {
		if _, stillExists := cfgs[filewalkerName]; !stillExists {
			fm.slogger.Log(context.TODO(), slog.LevelInfo,
				"deleting filewalker removed from config",
				"filewalker_name", filewalkerName,
			)
			fw.Delete()
			delete(fm.filewalkers, filewalkerName)
		}
	}

	fm.slogger.Log(context.TODO(), slog.LevelDebug,
		"completed filewalk config updates",
		"filewalker_count", len(fm.filewalkers),
	)
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

	fm.filewalkersLock.Lock()
	defer fm.filewalkersLock.Unlock()

	if len(req.FilewalkNames) > 0 {
		startedFilewalks := make([]string, 0)
		for _, filewalkName := range req.FilewalkNames {
			fw, fwFound := fm.filewalkers[filewalkName]
			if !fwFound {
				fm.slogger.Log(ctx, slog.LevelWarn,
					"filewalk request from control server contained unknown filewalk name",
					"filewalk_name", filewalkName,
				)
				continue
			}
			// Filewalks can take a while -- kick off the filewalk in the background so we don't
			// slow down control server processing.
			gowrapper.Go(context.TODO(), fm.slogger, func() { fw.Filewalk(context.TODO()) })
			startedFilewalks = append(startedFilewalks, filewalkName)
		}

		fm.slogger.Log(ctx, slog.LevelInfo,
			"kicked off specified filewalks per control server request",
			"requested_filewalks", req.FilewalkNames,
			"started_filewalks", startedFilewalks,
		)

		return nil
	}

	// If no filewalks were specified, kick off filewalks for all known filewalkers
	startedFilewalks := make([]string, 0)
	for filewalkName, fw := range fm.filewalkers {
		// Filewalks can take a while -- kick off the filewalk in the background so we don't
		// slow down control server processing.
		gowrapper.Go(context.TODO(), fm.slogger, func() { fw.Filewalk(context.TODO()) })
		startedFilewalks = append(startedFilewalks, filewalkName)
	}

	fm.slogger.Log(ctx, slog.LevelInfo,
		"kicked off all filewalks per control server request",
		"started_filewalks", startedFilewalks,
	)

	return nil
}
