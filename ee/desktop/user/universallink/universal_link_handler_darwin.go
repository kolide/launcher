//go:build darwin
// +build darwin

package universallink

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework AppKit

void StartUniversalLinkHandler(void);
*/
import "C"
import (
	"context"
	"log/slog"
)

var universalLinkInput chan string

type universalLinkHandler struct {
	slogger     *slog.Logger
	interrupted bool
	interrupt   chan struct{}
}

func NewUniversalLinkHandler(slogger *slog.Logger) *universalLinkHandler {
	universalLinkInput = make(chan string, 1)

	return &universalLinkHandler{
		slogger:   slogger.With("component", "universal_link_handler"),
		interrupt: make(chan struct{}),
	}
}

func (u *universalLinkHandler) Execute() error {
	C.StartUniversalLinkHandler()

	for {
		select {
		case i := <-universalLinkInput:
			if err := u.handleUniversalLinkRequest(i); err != nil {
				u.slogger.Log(context.TODO(), slog.LevelWarn,
					"could not handle universal link request",
					"err", err,
				)
			}
		case <-u.interrupt:
			u.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt, stopping",
			)
			return nil
		}
	}
}

func (u *universalLinkHandler) Interrupt(_ error) {
	u.slogger.Log(context.TODO(), slog.LevelInfo,
		"received interrupt",
	)

	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if u.interrupted {
		return
	}
	u.interrupted = true

	u.interrupt <- struct{}{}
}

// handleUniversalLinkRequest receives requests and logs them. In the future,
// it will validate them and forward them to launcher root.
func (u *universalLinkHandler) handleUniversalLinkRequest(requestUrl string) error {
	u.slogger.Log(context.TODO(), slog.LevelInfo,
		"received universal link request",
		"request_url", requestUrl,
	)

	// TODO: validate the request and forward it to launcher root

	return nil
}

//export handleUniversalLink
func handleUniversalLink(u *C.char) {
	universalLinkInput <- C.GoString(u)
}
