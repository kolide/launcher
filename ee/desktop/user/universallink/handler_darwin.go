//go:build darwin
// +build darwin

package universallink

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation

#include <stdbool.h>
#include <stdlib.h>

bool registerAppBundle(char *cAppBundlePath);
*/
import "C"
import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"unsafe"
)

// universalLinkHandler receives URLs from our AppDelegate in systray and forwards those requests
// to root launcher's localserver.
type universalLinkHandler struct {
	urlInput    chan string
	slogger     *slog.Logger
	interrupted atomic.Bool
	interrupt   chan struct{}
}

func NewUniversalLinkHandler(slogger *slog.Logger) (*universalLinkHandler, chan string) {
	urlInput := make(chan string, 1)
	return &universalLinkHandler{
		urlInput:  urlInput,
		slogger:   slogger.With("component", "universal_link_handler"),
		interrupt: make(chan struct{}),
	}, urlInput
}

func (u *universalLinkHandler) Execute() error {
	// Register self
	if err := register(); err != nil {
		u.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not register desktop app with Launch Services on startup",
			"err", err,
		)
	}
	for {
		select {
		case i := <-u.urlInput:
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
	u.slogger.Log(context.TODO(), slog.LevelDebug,
		"received interrupt",
	)

	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if u.interrupted.Swap(true) {
		return
	}

	u.interrupt <- struct{}{}
	close(u.urlInput)
}

func register() error {
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting current executable: %w", err)
	}

	// Point to `Kolide.app`
	currentExecutable = strings.TrimSuffix(currentExecutable, "/Contents/MacOS/launcher")

	currentExecutableCStr := C.CString(currentExecutable)
	defer C.free(unsafe.Pointer(currentExecutableCStr))

	success := C.registerAppBundle(currentExecutableCStr)
	if !success {
		return fmt.Errorf("could not register %s", currentExecutable)
	}

	return nil
}

// handleUniversalLinkRequest receives requests, validates them, and logs them.
// Nothing should be sending us universal link requests at the moment.
func (u *universalLinkHandler) handleUniversalLinkRequest(requestUrl string) error {
	// Parsing the URL also validates that we got a reasonable URL
	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return fmt.Errorf("parsing universal link request URL: %w", err)
	}

	u.slogger.Log(context.TODO(), slog.LevelWarn,
		"received unexpected universal link request",
		"request_host", parsedUrl.Host,
		"request_path", parsedUrl.Path,
		"request_query", parsedUrl.RawQuery,
	)

	return nil
}
