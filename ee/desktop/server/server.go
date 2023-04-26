// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/notify"
	"github.com/kolide/launcher/pkg/backoff"
)

type notificationSender interface {
	SendNotification(notify.Notification) error
}

// UserServer provides IPC for the root desktop runner to communicate with the user desktop processes.
// It allows the runner process to send notficaitons and commands to the desktop processes.
type UserServer struct {
	logger           log.Logger
	server           *http.Server
	listener         net.Listener
	shutdownChan     chan<- struct{}
	authToken        string
	socketPath       string
	notifier         notificationSender
	refreshListeners []func()
}

func New(logger log.Logger, authToken string, socketPath string, shutdownChan chan<- struct{}, notifier notificationSender) (*UserServer, error) {
	userServer := &UserServer{
		shutdownChan: shutdownChan,
		authToken:    authToken,
		logger:       log.With(logger, "component", "desktop_server"),
		socketPath:   socketPath,
		notifier:     notifier,
	}

	authedMux := http.NewServeMux()
	authedMux.HandleFunc("/shutdown", userServer.shutdownHandler)
	authedMux.HandleFunc("/ping", userServer.pingHandler)
	authedMux.HandleFunc("/notification", userServer.notificationHandler)
	authedMux.HandleFunc("/refresh", userServer.refreshHandler)

	userServer.server = &http.Server{
		Handler: userServer.authMiddleware(authedMux),
	}

	// remove existing socket
	if err := userServer.removeSocket(); err != nil {
		return nil, err
	}

	listener, err := listener(socketPath)
	if err != nil {
		return nil, err
	}
	userServer.listener = listener

	userServer.server.RegisterOnShutdown(func() {
		// remove socket on shutdown
		if err := userServer.removeSocket(); err != nil {
			level.Error(logger).Log("msg", "removing socket on shutdown", "err", err)
		}
	})

	return userServer, nil
}

func (s *UserServer) Serve() error {
	return s.server.Serve(s.listener)
}

func (s *UserServer) Shutdown(ctx context.Context) error {
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

func (s *UserServer) shutdownHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"msg": "shutting down"}` + "\n"))
	s.shutdownChan <- struct{}{}
}

func (s *UserServer) pingHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"pong": "Kolide"}` + "\n"))
}

func (s *UserServer) notificationHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	b, err := io.ReadAll(req.Body)
	if err != nil {
		level.Error(s.logger).Log("msg", "could not read body of notification request", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var notificationToSend notify.Notification
	if err := json.Unmarshal(b, &notificationToSend); err != nil {
		level.Error(s.logger).Log("msg", "could not decode notification request", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.notifier.SendNotification(notificationToSend); err != nil {
		level.Error(s.logger).Log("msg", "could not send notification", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *UserServer) refreshHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	s.notifyRefreshListeners()
}

// Registers a listener to be notified when status data should be refreshed
func (s *UserServer) RegisterRefreshListener(f func()) {
	s.refreshListeners = append(s.refreshListeners, f)
}

// Notifies all listeners to refresh their status data
func (s *UserServer) notifyRefreshListeners() {
	for _, listener := range s.refreshListeners {
		listener()
	}
}

func (s *UserServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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
func (s *UserServer) removeSocket() error {
	return backoff.WaitFor(func() error {
		return os.RemoveAll(s.socketPath)
	}, 5*time.Second, 1*time.Second)
}
