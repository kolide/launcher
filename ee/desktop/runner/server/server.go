package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kolide/kit/ulid"
)

// RunnerServer provides IPC for user desktop processes to communicate back to the root desktop runner.
// It allows the user process to notify and monitor the health of the runner process.
type RunnerServer struct {
	server                *http.Server
	listener              net.Listener
	slogger               *slog.Logger
	desktopProcAuthTokens map[string]string
	mutex                 sync.Mutex
	accelerator           requestAcclerator
	messenger             Messenger
}

const (
	HealthCheckEndpoint                = "/health"
	MenuOpenedEndpoint                 = "/menuopened"
	MessageEndpoint                    = "/message"
	controlRequestAccelerationInterval = 5 * time.Second
	controlRequestAcclerationDuration  = 1 * time.Minute
)

type requestAcclerator interface {
	// SetControlRequestIntervalOverride sets the interval for control requests
	SetControlRequestIntervalOverride(time.Duration, time.Duration)
	// SetDistributedForwardingIntervalOverride sets the interval for osquery
	SetDistributedForwardingIntervalOverride(time.Duration, time.Duration)
}

type Messenger interface {
	SendMessage(method string, params interface{}) error
}

func New(slogger *slog.Logger,
	accelerator requestAcclerator,
	messenger Messenger) (*RunnerServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("creating net listener: %w", err)
	}

	rs := &RunnerServer{
		listener:              listener,
		slogger:               slogger,
		desktopProcAuthTokens: make(map[string]string),
		accelerator:           accelerator,
		messenger:             messenger,
	}

	if rs.slogger == nil {
		return nil, errors.New("slogger cannot be nil")
	}

	rs.slogger = slogger.With("component", "desktop_runner_root_server")

	mux := http.NewServeMux()

	// health check endpoint
	mux.HandleFunc(HealthCheckEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}
	})

	// menu opened endpoint
	mux.HandleFunc(MenuOpenedEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		// accelerate control server requests
		rs.accelerator.SetControlRequestIntervalOverride(controlRequestAccelerationInterval, controlRequestAcclerationDuration)
		// accelerate osquery requests
		rs.accelerator.SetDistributedForwardingIntervalOverride(controlRequestAccelerationInterval, controlRequestAcclerationDuration)
	})

	mux.Handle(MessageEndpoint, http.HandlerFunc(rs.sendMessage))

	rs.server = &http.Server{
		Handler: rs.authMiddleware(mux),
	}

	return rs, err
}

func (ms *RunnerServer) Serve() error {
	return ms.server.Serve(ms.listener)
}

func (ms *RunnerServer) Shutdown(ctx context.Context) error {
	return ms.server.Shutdown(ctx)
}

// RegisterClient registers a desktop proc with the server under the provided key.
// If a desktop proc already exists with the provided key, it will be replaced and
// no longer recognized.
// Returns the generated auth token.
func (ms *RunnerServer) RegisterClient(key string) string {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	token := ulid.New()
	ms.desktopProcAuthTokens[key] = token
	return token
}

func (ms *RunnerServer) DeRegisterClient(key string) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	_, ok := ms.desktopProcAuthTokens[key]
	if ok {
		delete(ms.desktopProcAuthTokens, key)
	}
}

func (ms *RunnerServer) Url() string {
	return fmt.Sprintf("http://%s", ms.listener.Addr().String())
}

func (ms *RunnerServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.Split(r.Header.Get("Authorization"), "Bearer ")

		if len(authHeader) != 2 {
			ms.slogger.Log(r.Context(), slog.LevelDebug,
				"malformed authorization header",
			)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if !ms.isAuthTokenValid(authHeader[1]) {
			ms.slogger.Log(r.Context(), slog.LevelDebug,
				"invalid desktop auth token",
			)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ms *RunnerServer) isAuthTokenValid(authToken string) bool {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	for _, v := range ms.desktopProcAuthTokens {
		if v == authToken {
			return true
		}
	}

	return false
}

func (ms *RunnerServer) sendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		ms.slogger.Log(r.Context(), slog.LevelError,
			"no request body",
		)

		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	message := struct {
		Method string      `json:"method"`
		Params interface{} `json:"params"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		ms.slogger.Log(r.Context(), slog.LevelError,
			"could not decode request body",
			"err", err,
		)

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if message.Method == "" {
		ms.slogger.Log(r.Context(), slog.LevelError,
			"does not include method property",
		)

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := ms.messenger.SendMessage(message.Method, message.Params); err != nil {
		ms.slogger.Log(r.Context(), slog.LevelError,
			"error sending message",
			"err", err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}
