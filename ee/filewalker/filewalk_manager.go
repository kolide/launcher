package filewalker

import (
	"log/slog"
	"sync/atomic"

	"github.com/kolide/launcher/ee/agent/types"
)

type FilewalkManager struct {
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
		k:           k,
		cfgStore:    k.FilewalkConfigStore(),
		slogger:     slogger.With("component", "filewalker"),
		interrupt:   make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		interrupted: &atomic.Bool{},
	}
}

func (fm *FilewalkManager) Execute() error {
	<-fm.interrupt
	return nil
}

func (fm *FilewalkManager) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if fm.interrupted.Swap(true) {
		return
	}

	fm.interrupt <- struct{}{}
}

// Ping satisfies the control.subscriber interface -- the manager subscribes to changes to
// the filewalk_config subsystem.
func (fm *FilewalkManager) Ping() {}
