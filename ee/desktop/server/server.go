// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/backoff"
)

type DesktopServer struct {
	logger           log.Logger
	server           *http.Server
	listener         net.Listener
	shutdownChan     chan<- struct{}
	authToken        string
	socketPath       string
	refreshListeners []func()
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
	authedMux.HandleFunc("/ping", desktopServer.pingHandler)
	authedMux.HandleFunc("/refresh", desktopServer.refreshHandler)

	desktopServer.server = &http.Server{
		Handler: desktopServer.authMiddleware(authedMux),
	}

	// remove existing socket
	if err := desktopServer.removeSocket(); err != nil {
		return nil, err
	}

	listener, err := listener(socketPath)
	if err != nil {
		return nil, err
	}
	desktopServer.listener = listener

	desktopServer.server.RegisterOnShutdown(func() {
		// remove socket on shutdown
		if err := desktopServer.removeSocket(); err != nil {
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
	// on non-windows this gives an error
	if runtime.GOOS == "windows" {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (s *DesktopServer) shutdownHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"msg": "shutting down"}` + "\n"))
	s.shutdownChan <- struct{}{}
}

func (s *DesktopServer) pingHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"pong": "Kolide"}` + "\n"))
}

func (s *DesktopServer) refreshHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	s.notifyRefreshListeners()
}

// Registers a listener to be notified when status data should be refreshed
func (s *DesktopServer) RegisterRefreshListener(f func()) {
	s.refreshListeners = append(s.refreshListeners, f)
}

// Notifies all listeners to refresh their status data
func (s *DesktopServer) notifyRefreshListeners() {
	for _, listener := range s.refreshListeners {
		listener()
	}
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

// removeSocket is a helper function to remove the socket file. The reason it exists is that
// on windows, you can't delete a file that is opened by another resource. When the server
// shuts down, there is some lag time before the file is release, this can cause errors
// when trying to delete the file.
func (s *DesktopServer) removeSocket() error {
	return backoff.WaitFor(func() error {
		return os.RemoveAll(s.socketPath)
	}, 5*time.Second, 1*time.Second)
}
