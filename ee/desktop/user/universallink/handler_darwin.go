//go:build darwin
// +build darwin

package universallink

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/localserver"
)

const (
	universalLinkPrefix = "/launcher/applinks/"
	requestTimeout      = 40 * time.Second // localserver timeout plus a little extra buffer
)

// universalLinkHandler receives URLs from our AppDelegate in systray and forwards those requests
// to root launcher's localserver.
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

func register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting current executable: %w", err)
	}

	// If we're running the originally-installed version of launcher, no need to register
	if strings.HasPrefix(currentExecutable, "/usr/local") {
		return nil
	}

	// Point to `Kolide.app`
	currentExecutable = strings.TrimSuffix(currentExecutable, "/Contents/MacOS/launcher")

	// Run lsregister against this path
	lsregisterCmd, err := allowedcmd.Lsregister(ctx, currentExecutable)
	if err != nil {
		return fmt.Errorf("creating lsregister %s command: %w", currentExecutable, err)
	}

	if out, err := lsregisterCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running lsregister %s: output `%s`, err: %w", currentExecutable, string(out), err)
	}

	return nil
}

// handleUniversalLinkRequest receives requests, validates them, and forwards them
// to launcher root's localserver.
func (u *universalLinkHandler) handleUniversalLinkRequest(requestUrl string) error {
	// Parsing the URL also validates that we got a reasonable URL
	parsedUrl, err := url.Parse(requestUrl)
	if err != nil {
		return fmt.Errorf("parsing universal link request URL: %w", err)
	}

	origin := fmt.Sprintf("%s://%s", parsedUrl.Scheme, parsedUrl.Host)
	requestPath := strings.TrimPrefix(parsedUrl.Path, universalLinkPrefix)
	requestQuery := parsedUrl.RawQuery

	// Forward the request to each potential launcher root port, in parallel
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, p := range localserver.PortList {
		p := p
		wg.Add(1)

		gowrapper.Go(ctx, u.slogger, func() {
			defer wg.Done()
			if err := forwardRequest(ctx, p, requestPath, requestQuery, origin); err != nil {
				if strings.Contains(err.Error(), "connection refused") {
					// Launcher not running on that port -- no need to log the error
					return
				}
				u.slogger.Log(ctx, slog.LevelWarn,
					"could not forward universal link request",
					"port", p,
					"err", err,
				)
				return
			}

			// Success!
			u.slogger.Log(ctx, slog.LevelDebug,
				"successfully forwarded universal link request",
				"port", p,
			)
		},
			func(r any) {}, // no special behavior needed after recovering from panic
		)
	}

	wg.Wait()

	return nil
}

func forwardRequest(ctx context.Context, port int, requestPath string, requestQuery string, requestOrigin string) error {
	// Construct forwarded request, using URL as origin
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/%s?%s", port, requestPath, requestQuery), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Origin", requestOrigin)

	// Forward request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("forwarding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
