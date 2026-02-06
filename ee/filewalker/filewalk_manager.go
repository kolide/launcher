package filewalker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
)

type FilewalkManager struct {
	filewalkers     map[string]filewalker
	filewalkersLock *sync.Mutex

	// Internals
	k       types.Knapsack
	slogger *slog.Logger

	// Handle actor shutdown
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func New(k types.Knapsack, slogger *slog.Logger) *FilewalkManager {
	return &FilewalkManager{
		filewalkers:     make(map[string]filewalker),
		filewalkersLock: &sync.Mutex{},
		k:               k,
		slogger:         slogger.With("component", "filewalker"),
		interrupt:       make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		interrupted:     &atomic.Bool{},
	}
}

func (fm *FilewalkManager) Execute() error {
	// Init filewalkers
	cfgs := make([]filewalkConfig, 0) // TODO RM: pull from storage once available
	fm.filewalkersLock.Lock()
	for _, cfg := range cfgs {
		fm.filewalkers[cfg.name] = *newFilewalker(cfg, fm.slogger)
	}
	for _, fw := range fm.filewalkers {
		gowrapper.Go(context.TODO(), fm.slogger, fw.Work)
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

func (fm *FilewalkManager) Update(data io.Reader) error {
	return errors.New("not implemented")
}
