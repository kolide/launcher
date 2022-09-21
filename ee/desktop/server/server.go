// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type DesktopServer struct {
	logger       log.Logger
	server       *http.Server
	listener     net.Listener
	shutdownChan chan<- struct{}
	authToken    string
}

func New(logger log.Logger, authToken string, socketPath string, shutdownChan chan<- struct{}) (*DesktopServer, error) {
	desktopServer := &DesktopServer{
		shutdownChan: shutdownChan,
		authToken:    authToken,
		logger:       log.With(logger, "component", "desktop_server"),
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
	return s.server.Shutdown(ctx)
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
