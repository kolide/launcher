package localserver

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestIdHandler(t *testing.T) {
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
			server := testServer(t, &logBytes)

			req, err := http.NewRequest("", "", nil)
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestIdHandlerFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Empty(t, logBytes.String())
			assert.Equal(t, http.StatusOK, rr.Code)

			// convert the response to a struct
			var response requestIdsResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))

			assert.GreaterOrEqual(t, len(response.ConsoleUsers), 1, "should have at least one console user")

		})
	}
}

func testServer(t *testing.T, logBytes *bytes.Buffer) *localServer {
	server, err := New(log.NewLogfmtLogger(logBytes), func() (*rsa.PrivateKey, error) { return nil, nil }, "")
	require.NoError(t, err)
	return server
}
