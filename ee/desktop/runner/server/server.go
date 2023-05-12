package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
)

// RunnerServer provides IPC for user desktop processes to communicate back to the root desktop runner.
// It allows the user process to notify and monitor the health of the runner process.
type RunnerServer struct {
	server                          *http.Server
	listener                        net.Listener
	logger                          log.Logger
	desktopProcAuthTokens           map[string]string
	mutex                           sync.Mutex
	controlRequestIntervalOverrider controlRequestIntervalOverrider
}

const (
	HealthCheckEndpoint                = "/health"
	MenuOpenedEndpoint                 = "/menuopened"
	controlRequestAccelerationInterval = 5 * time.Second
	controlRequestAcclerationDuration  = 1 * time.Minute
)

type controlRequestIntervalOverrider interface {
	SetControlRequestIntervalOverride(time.Duration, time.Duration)
}

func New(logger log.Logger, controlRequestIntervalOverrider controlRequestIntervalOverrider) (*RunnerServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("creating net listener: %w", err)
	}

	rs := &RunnerServer{
		listener:                        listener,
		logger:                          logger,
		desktopProcAuthTokens:           make(map[string]string),
		controlRequestIntervalOverrider: controlRequestIntervalOverrider,
	}

	if rs.logger != nil {
		rs.logger = log.With(rs.logger, "component", "desktop_runner_root_server")
	}

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

		controlRequestIntervalOverrider.SetControlRequestIntervalOverride(controlRequestAccelerationInterval, controlRequestAcclerationDuration)
	})

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
// Returns the generated auth token.
func (ms *RunnerServer) RegisterClient(key string) string {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	v, ok := ms.desktopProcAuthTokens[key]
	if ok {
		return v
	}

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
			level.Debug(ms.logger).Log("msg", "malformed authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		ms.mutex.Lock()
		defer ms.mutex.Unlock()

		key := ""
		for k, v := range ms.desktopProcAuthTokens {
			if v == authHeader[1] {
				key = k
				break
			}
		}

		if key == "" {
			level.Debug(ms.logger).Log("msg", "no key found for desktop auth token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
