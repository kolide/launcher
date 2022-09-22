// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type DesktopServer struct {
	logger       log.Logger
	server       *http.Server
	listener     net.Listener
	shutdownChan chan<- struct{}
	authToken    string
	socketPath   string
}

func New(logger log.Logger, authToken string, socketPath string, shutdownChan chan<- struct{}) (*DesktopServer, error) {
	desktopServer := &DesktopServer{
		shutdownChan: shutdownChan,
		authToken:    authToken,
		logger:       log.With(logger, "component", "desktop_server"),
		socketPath:   socketPath,
	}

	authedMux := http.NewServeMux()
	authedMux.HandleFunc("/shutdown", desktopServer.shutdownHandler)

	mux := http.NewServeMux()
	mux.Handle("/", desktopServer.authMiddleware(authedMux))

	desktopServer.server = &http.Server{
		Handler: mux,
	}

	// remove existing socket
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, err
	}

	listener, err := listener(socketPath)
	if err != nil {
		return nil, err
	}
	desktopServer.listener = listener

	desktopServer.server.RegisterOnShutdown(func() {
		// remove socket on shutdown
		if err := os.RemoveAll(socketPath); err != nil {
			level.Error(logger).Log("msg", "removing socket on shutdown", "err", err)
		}
	})

	return desktopServer, nil
}

func (s *DesktopServer) Serve() error {
	return s.server.Serve(s.listener)
}

func (s *DesktopServer) Shutdown(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	if err != nil {
		return err
	}

	// on windows we need to expliclty close the listener
	// on nonwindows this gives an error
	if runtime.GOOS == "windows" {
		return s.listener.Close()
	}

	// on windows you cannot remove a file if it is open by another resource
	// it takes a bit of time between the server shutdown and the socket file
	// being freed up, so we wait for it to be removed
	return waitForRemoveFile(s.socketPath, 1*time.Second, 5*time.Second)
}

func (s *DesktopServer) shutdownHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"msg\": \"shutting down\"}"))
	s.shutdownChan <- struct{}{}
}

func (s *DesktopServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		authHeader := strings.Split(r.Header.Get("Authorization"), "Bearer ")

		if len(authHeader) != 2 {
			level.Debug(s.logger).Log("msg", "malformed authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if authHeader[1] != s.authToken {
			level.Debug(s.logger).Log("msg", "invalid authorization token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// waitForRemoveFile tries to remove a file on the given interval, returns error if time out exceeded
func waitForRemoveFile(path string, interval, timeout time.Duration) error {
	intervalTicker := time.NewTicker(interval)
	defer intervalTicker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	f := func() bool {
		return os.RemoveAll(path) == nil
	}

	if f() {
		return nil
	}

	for {
		select {
		case <-timeoutTimer.C:
			return fmt.Errorf("timeout waiting for file deletion: %s", path)
		case <-intervalTicker.C:
			if f() {
				return nil
			}
		}
	}
}
