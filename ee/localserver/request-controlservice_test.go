package localserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/agent/types/mocks"

	"github.com/stretchr/testify/require"
)

func Test_localServer_requestAccelerateControlFunc(t *testing.T) {
	t.Parallel()

	defaultMockKnapsack := func() types.Knapsack {
		m := mocks.NewKnapsack(t)
		m.On("ConfigStore").Return(storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String()))
		m.On("KolideServerURL").Return("localhost")
		return m
	}

	tests := []struct {
		name                  string
		expectedHttpStatus    int
		expectedInterval      time.Duration
		body                  map[string]string
		httpErrStr, logErrStr string
		mockKnapsack          func() types.Knapsack
	}{
		{
			name:               "happy path",
			expectedHttpStatus: http.StatusOK,
			body: map[string]string{
				"interval": "250ms",
				"duration": "1s",
			},
			expectedInterval: 250 * time.Millisecond,
			mockKnapsack: func() types.Knapsack {
				m := mocks.NewKnapsack(t)
				m.On("ConfigStore").Return(storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String()))
				m.On("KolideServerURL").Return("localhost")
				m.On("SetControlRequestIntervalOverride", 250*time.Millisecond, 1*time.Second)
				return m
			},
		},
		{
			name:               "no body",
			expectedHttpStatus: http.StatusBadRequest,
			httpErrStr:         "request body is nil",
			mockKnapsack:       defaultMockKnapsack,
		},
		{
			name:               "bad interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "blah",
				"duration": "1s",
			},
			httpErrStr:   "error parsing interval",
			mockKnapsack: defaultMockKnapsack,
		},
		{
			name:               "empty interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "",
				"duration": "1s",
			},
			httpErrStr:   "error parsing interval",
			mockKnapsack: defaultMockKnapsack,
		},
		{
			name:               "no interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"duration": "1s",
			},
			httpErrStr:   "error parsing interval",
			mockKnapsack: defaultMockKnapsack,
		},
		{
			name:               "bad duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "blah",
			},
			httpErrStr:   "error parsing duration",
			mockKnapsack: defaultMockKnapsack,
		},
		{
			name:               "empty duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "",
			},
			httpErrStr:   "error parsing duration",
			mockKnapsack: defaultMockKnapsack,
		},
		{
			name:               "no duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
			},
			httpErrStr:   "error parsing duration",
			mockKnapsack: defaultMockKnapsack,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var k types.Knapsack
			if tt.mockKnapsack != nil {
				k = tt.mockKnapsack()
			}

			var logBytes bytes.Buffer
			server := testServer(t, k, &logBytes)

			req, err := http.NewRequest("", "", nil)
			if tt.body != nil {
				req, err = http.NewRequest("", "", bytes.NewBuffer(mustMarshal(t, tt.body)))
			}
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestAccelerateControlFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedHttpStatus, rr.Code)

			if tt.httpErrStr != "" {
				require.Contains(t, rr.Body.String(), tt.httpErrStr)
			}

			if tt.logErrStr != "" {
				require.Contains(t, logBytes.String(), tt.logErrStr)
			}
		})
	}
}
