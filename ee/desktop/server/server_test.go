// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesktopServer_AuthMiddleware(t *testing.T) {
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

			s := &DesktopServer{}

			h := s.authMiddleware(makeTestHandler(t))

			req, err := http.NewRequest("GET", "https://127.0.0.1:8080", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
		})
	}
}

func makeTestHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.String()))
	})
}

func makeRequest(t *testing.T) *http.Request {
	req, err := http.NewRequest("GET", "https://127.0.0.1:8080", nil)
	require.NoError(t, err)

	return req
}
