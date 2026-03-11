package server

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// ioCompletionProcessor will continue to run forever until the process (go test in this case) exits,
	// so we need goleak to ignore that one.
	goleak.VerifyTestMain(m, goleak.IgnoreAnyFunction("github.com/Microsoft/go-winio.ioCompletionProcessor"))
}

const validAuthHeader = "test-auth-header"

func TestUserServer_authMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		loggedErr  string
		authHeader string
	}{
		{
			name:      "malformed_authorization_header",
			loggedErr: "malformed authorization header",
		},
		{
			name:       "invalid_authorization_token",
			loggedErr:  "invalid authorization token",
			authHeader: "Bearer invalid",
		},
		{
			name:       "valid_token",
			authHeader: fmt.Sprintf("Bearer %s", validAuthHeader),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			server, _ := testServer(t, validAuthHeader, testSocketPath(t), &logBytes)

			req, err := http.NewRequest("GET", "https://127.0.0.1:8080", nil) //nolint:noctx // We don't care about this in tests
			require.NoError(t, err)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			handler := server.authMiddleware(testHandler())
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
			}

			require.NoError(t, server.Shutdown(t.Context()))

			time.Sleep(5 * time.Second) // wait for removeSocket to finish
		})
	}
}

func TestUserServer_shutdownHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			server, shutdownChan := testServer(t, validAuthHeader, testSocketPath(t), &logBytes)

			go func() {
				<-shutdownChan
			}()

			req, err := http.NewRequestWithContext(t.Context(), "", "", nil) //nolint:noctx // We don't care about this in tests
			require.NoError(t, err)

			handler := http.HandlerFunc(server.shutdownHandler)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Empty(t, logBytes.String())
			assert.Equal(t, http.StatusOK, rr.Code)

			require.NoError(t, server.Shutdown(t.Context()))

			time.Sleep(5 * time.Second) // wait for removeSocket to finish
		})
	}
}

func testHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.String()))
	})
}

func testServer(t *testing.T, authHeader, socketPath string, logBytes *bytes.Buffer) (*UserServer, chan struct{}) {
	shutdownChan := make(chan struct{})

	slogger := slog.New(slog.NewTextHandler(logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	server, err := New(slogger, authHeader, socketPath, shutdownChan, make(chan<- struct{}), nil)
	require.NoError(t, err)
	return server, shutdownChan
}

func testSocketPath(t *testing.T) string {
	socketFileName := strings.ReplaceAll(t.Name(), "/", "_")

	// using t.TempDir() creates a file path too long for a unix socket
	socketPath := filepath.Join(os.TempDir(), socketFileName)
	// truncate socket path to max length
	if len(socketPath) > 103 {
		socketPath = socketPath[:103]
	}

	if runtime.GOOS == "windows" {
		socketPath = fmt.Sprintf(`\\.\pipe\%s`, socketFileName)
	}

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(socketPath))
	})

	return socketPath
}
