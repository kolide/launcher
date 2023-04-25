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
	"github.com/kolide/launcher/pkg/agent/types"
)

type RootServer struct {
	server                *http.Server
	listener              net.Listener
	logger                log.Logger
	desktopProcAuthTokens map[string]string
	mutex                 sync.Mutex
	knapsack              types.Knapsack
}

const (
	HealthCheckEndpoint = "/health"
	MenuOpenedEndpoint  = "/menuopened"
)

func New(logger log.Logger, k types.Knapsack) (*RootServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}

	ms := &RootServer{
		listener:              listener,
		logger:                logger,
		desktopProcAuthTokens: make(map[string]string),
		knapsack:              k,
	}

	if ms.logger != nil {
		ms.logger = log.With(ms.logger, "component", "desktop_runner_root_server")
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

		ms.knapsack.SetControlRequestIntervalOverride(5*time.Second, 1*time.Minute)
	})

	ms.server = &http.Server{
		Handler: ms.authMiddleware(mux),
	}

	return ms, err
}

func (ms *RootServer) Serve() error {
	return ms.server.Serve(ms.listener)
}

func (ms *RootServer) Shutdown(ctx context.Context) error {
	return ms.server.Shutdown(ctx)
}

// RegisterClient registers a desktop proc with the server under the provided key.
// Returns the generated auth token.
func (ms *RootServer) RegisterClient(key string) string {
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

func (ms *RootServer) DeRegisterClient(key string) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	_, ok := ms.desktopProcAuthTokens[key]
	if ok {
		delete(ms.desktopProcAuthTokens, key)
	}
}

func (ms *RootServer) Url() string {
	return fmt.Sprintf("http://%s", ms.listener.Addr().String())
}

func (ms *RootServer) authMiddleware(next http.Handler) http.Handler {
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
