// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validAuthHeader = "test-auth-header"

func TestDesktopServer_authMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		loggedErr  string
		authHeader string
	}{
		{
			name:      "malformed authorization header",
			loggedErr: "malformed authorization header",
		},
		{
			name:       "invalid authorization token",
			loggedErr:  "invalid authorization token",
			authHeader: "Bearer invalid",
		},
		{
			name:       "valid token",
			authHeader: fmt.Sprintf("Bearer %s", validAuthHeader),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			server, _ := testServer(t, validAuthHeader, testSocketPath(t, tt.name), &logBytes)

			req, err := http.NewRequest("GET", "https://127.0.0.1:8080", nil)
			require.NoError(t, err)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			handler := server.authMiddleware(testHandler(t))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
			}

			server.Shutdown(context.Background())
		})
	}
}

func TestDesktopServer_shutdownHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			server, shutdownChan := testServer(t, validAuthHeader, testSocketPath(t, tt.name), &logBytes)

			go func() {
				<-shutdownChan
			}()

			req, err := http.NewRequest("", "", nil)
			require.NoError(t, err)

			handler := http.HandlerFunc(server.shutdownHandler)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Empty(t, logBytes.String())
			assert.Equal(t, http.StatusOK, rr.Code)
		})
	}
}

func testHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.String()))
	})
}

func testServer(t *testing.T, authHeader, socketPath string, logBytes *bytes.Buffer) (*DesktopServer, chan struct{}) {
	shutdownChan := make(chan struct{})

	server, err := New(log.NewLogfmtLogger(logBytes), authHeader, socketPath, shutdownChan)
	require.NoError(t, err)
	return server, shutdownChan
}

func testSocketPath(t *testing.T, testName string) string {
	// using t.TempDir() creates a file path too long for a unix socket
	socketPath := filepath.Join(os.TempDir(), testName)
	if runtime.GOOS == "windows" {
		socketPath = fmt.Sprintf(`\\.\pipe\%s`, testName)
	}

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(socketPath))
	})

	return socketPath
}
