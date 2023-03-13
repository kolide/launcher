package localserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestControlServerFetchInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		intervalString string
		errStr         string
	}{
		{
			intervalString: "1s",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			//go:generate mockery --name Querier
			// https://github.com/vektra/mockery <-- cli tool to generate mocks for usage with testify
			mockControlServer := mocks.NewControlServer(t)
			mockControlServer.On("UpdateRequestInterval", mustParseDuration(t, tt.intervalString)).Return(nil)

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)
			server.controlServer = mockControlServer

			jsonBytes, err := json.Marshal(map[string]string{
				"interval": tt.intervalString,
			})
			req, err := http.NewRequest("", "", bytes.NewBuffer(jsonBytes))
			require.NoError(t, err)

			// queryParams := req.URL.Query()
			// queryParams.Add("query", tt.query)
			// req.URL.RawQuery = queryParams.Encode()

			handler := http.HandlerFunc(server.requestControlServerFetchIntervalFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			// if tt.mockQueryResult != nil {
			// 	require.Equal(t, mustMarshal(t, tt.mockQueryResult), rr.Body.Bytes())
			// 	return
			// }

			// require.Contains(t, rr.Body.String(), tt.errStr)
		})
	}
}

func mustParseDuration(t *testing.T, s string) time.Duration {
	duration, err := time.ParseDuration(s)
	require.NoError(t, err)
	return duration
}
