package localserver

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestAccelerateControlFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		expectedHttpStatus    int
		body                  map[string]string
		httpErrStr, logErrStr string
		mockControlService    func() controlService
	}{
		{
			name:               "happy path",
			expectedHttpStatus: http.StatusOK,
			body: map[string]string{
				"interval": "250ms",
				"duration": "1s",
			},
			mockControlService: func() controlService {
				m := mocks.NewControlService(t)
				m.On("AccelerateRequestInterval", 250*time.Millisecond, 1*time.Second).Return(nil)
				return m
			},
		},
		{
			name:               "acceleration failed",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "1s",
			},
			mockControlService: func() controlService {
				m := mocks.NewControlService(t)
				m.On("AccelerateRequestInterval", 250*time.Millisecond, 1*time.Second).Return(errors.New("some acceleration error"))
				return m
			},
			logErrStr: "some acceleration error",
		},
		{
			name:               "no body",
			expectedHttpStatus: http.StatusBadRequest,
			httpErrStr:         "request body is nil",
		},
		{
			name:               "no control service",
			expectedHttpStatus: http.StatusBadRequest,
			httpErrStr:         "control service not configured",
			body:               map[string]string{},
		},
		{
			name:               "bad interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "blah",
				"duration": "1s",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing interval",
		},
		{
			name:               "empty interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "",
				"duration": "1s",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing interval",
		},
		{
			name:               "no interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"duration": "1s",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing interval",
		},
		{
			name:               "bad duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "blah",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing duration",
		},
		{
			name:               "empty duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing duration",
		},
		{
			name:               "no duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
			},
			mockControlService: func() controlService { return mocks.NewControlService(t) },
			httpErrStr:         "error parsing duration",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)

			if tt.mockControlService != nil {
				server.controlService = tt.mockControlService()
			}

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
