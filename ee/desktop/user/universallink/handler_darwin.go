//go:build darwin
// +build darwin

package universallink

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

const universalLinkPrefix = "/launcher/applinks"

type universalLinkHandler struct {
	urlInput    chan string
	slogger     *slog.Logger
	interrupted bool
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
	u.slogger.Log(context.TODO(), slog.LevelInfo,
		"received interrupt",
	)

	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if u.interrupted {
		return
	}
	u.interrupted = true

	u.interrupt <- struct{}{}
	close(u.urlInput)
}

// handleUniversalLinkRequest receives requests and logs them. In the future,
// it will validate them and forward them to launcher root.
func (u *universalLinkHandler) handleUniversalLinkRequest(requestUrl string) error {
	// Parsing the URL also validates that we got a reasonable URL
	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return fmt.Errorf("parsing universal link request URL: %w", err)
	}

	origin := parsedUrl.Host
	requestPath := strings.TrimPrefix(parsedUrl.Path, universalLinkPrefix)
	requestQuery := parsedUrl.RawQuery

	u.slogger.Log(context.TODO(), slog.LevelInfo,
		"received universal link request",
		"origin", origin,
		"request_path", requestPath,
		"request_query", requestQuery,
	)

	// TODO: forward the request to launcher root

	return nil
}
