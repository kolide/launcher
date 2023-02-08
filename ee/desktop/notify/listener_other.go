//go:build !linux
// +build !linux

package notify

import (
	"github.com/go-kit/kit/log"
)

type noopListener struct {
	logger    log.Logger
	interrupt chan struct{}
}

func newOsSpecificListener(logger log.Logger) (*noopListener, error) {
	return &noopListener{
		logger:    logger,
		interrupt: make(chan struct{}),
	}, nil
}

func (n *noopListener) Listen() error {
	for {
		select {
		case <-n.interrupt:
			return nil
		}
	}
}

func (n *noopListener) Interrupt(err error) {
	n.interrupt <- struct{}{}
}
