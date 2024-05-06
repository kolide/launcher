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

// customProtocolHandler receives requests from the browser that cannot be sent
// directly to localserver; it processes and forwards them. Currently, this exists
// only to ensure Safari support for device trust. Custom protocol handling requires
// a running process for the given user, so this actor must run in launcher desktop.
type customProtocolHandler struct {
	slogger     *slog.Logger
	interrupted bool
	interrupt   chan struct{}
}

func NewCustomProtocolHandler(slogger *slog.Logger) *customProtocolHandler {
	urlInput = make(chan string, 1)

	return &customProtocolHandler{
		slogger:   slogger.With("component", "custom_protocol_handler"),
		interrupt: make(chan struct{}),
	}
}

func (c *customProtocolHandler) Execute() error {
	C.StartURLHandler()

	for {
		select {
		case i := <-urlInput:
			if err := c.handleCustomProtocolRequest(i); err != nil {
				c.slogger.Log(context.TODO(), slog.LevelWarn,
					"could not handle custom protocol request",
					"err", err,
				)
			}
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

// handleCustomProtocolRequest receives requests and logs them. In the future,
// it will validate them and forward them to launcher root.
func (c *customProtocolHandler) handleCustomProtocolRequest(requestUrl string) error {
	c.slogger.Log(context.TODO(), slog.LevelInfo,
		"received custom protocol request",
		"request_url", requestUrl,
	)

	// TODO: validate the request and forward it to launcher root

	return nil
}

//export handleURL
func handleURL(u *C.char) {
	urlInput <- C.GoString(u)
}
