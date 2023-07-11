//go:build !windows
// +build !windows

package powereventwatcher

import (
	"github.com/go-kit/kit/log"
)

type noOpPowerEventWatcher struct {
	interrupt chan struct{}
}

func New(_ log.Logger) (*noOpPowerEventWatcher, error) {
	return &noOpPowerEventWatcher{
		interrupt: make(chan struct{}),
	}, nil
}

func (n *noOpPowerEventWatcher) Execute() error {
	<-n.interrupt
	return nil
}

func (n *noOpPowerEventWatcher) Interrupt(_ error) {
	n.interrupt <- struct{}{}
}
