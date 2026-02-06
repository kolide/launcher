package filewalker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
)

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
	for _, cfg := range cfgs {
		fm.filewalkers[cfg.name] = newFilewalker(cfg, fm.k.FilewalkResultsStore(), fm.slogger)
		gowrapper.Go(context.TODO(), fm.slogger, fm.filewalkers[cfg.name].Work)
	}
	fm.filewalkersLock.Unlock()

	// Wait for shutdown, then clean up all filewalkers
	<-fm.interrupt
	fm.filewalkersLock.Lock()
	defer fm.filewalkersLock.Unlock()
	for _, fw := range fm.filewalkers {
		fw.Stop()
	}
	return nil
}

func (fm *FilewalkManager) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if fm.interrupted.Swap(true) {
		return
	}

	fm.interrupt <- struct{}{}
}

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
		} else {
			// Add the new filewalker
			fm.filewalkers[cfg.name] = newFilewalker(cfg, fm.k.FilewalkResultsStore(), fm.slogger)
			gowrapper.Go(context.TODO(), fm.slogger, fm.filewalkers[cfg.name].Work)
		}
	}

	// Now, check to see if we need to shut down and delete any filewalkers
	for filewalkerName, fw := range fm.filewalkers {
		if _, stillExists := cfgs[filewalkerName]; !stillExists {
			fw.Delete()
			delete(fm.filewalkers, filewalkerName)
		}
	}
}
