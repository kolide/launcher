//go:build darwin
// +build darwin

package customprotocol

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework AppKit
#include "handler.h"
*/
import "C"
import (
	"context"
	"log/slog"
)

var urlInput chan string

type customProtocolHandler struct {
	slogger     *slog.Logger
	interrupted bool
	interrupt   chan struct{}
}

func NewCustomProtocolHandler(slogger *slog.Logger) *customProtocolHandler {
	return &customProtocolHandler{
		slogger:   slogger.With("component", "custom_protocol_handler"),
		interrupt: make(chan struct{}),
	}
}

func (c *customProtocolHandler) Execute() error {
	urlInput = make(chan string, 1)
	go C.StartURLHandler()

	for {
		select {
		case i := <-urlInput:
			c.slogger.Log(context.TODO(), slog.LevelInfo,
				"got input from URL handler!",
				"input", i,
			)
		case <-c.interrupt:
			c.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt, stopping",
			)
			return nil
		}
	}
}

func (c *customProtocolHandler) Interrupt(_ error) {
	c.slogger.Log(context.TODO(), slog.LevelInfo,
		"received interrupt",
	)

	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if c.interrupted {
		return
	}
	c.interrupted = true

	c.interrupt <- struct{}{}
}

//export handleURL
func handleURL(u *C.char) {
	urlInput <- C.GoString(u)
}
