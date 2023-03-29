package localserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/flags/mocks"

	"github.com/stretchr/testify/require"
)

func Test_localServer_requestAccelerateControlFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		expectedHttpStatus    int
		expectedInterval      time.Duration
		body                  map[string]string
		httpErrStr, logErrStr string
		mockFlags             func() flags.Flags
	}{
		{
			name:               "happy path",
			expectedHttpStatus: http.StatusOK,
			body: map[string]string{
				"interval": "250ms",
				"duration": "1s",
			},
			expectedInterval: 250 * time.Millisecond,
			mockFlags: func() flags.Flags {
				m := mocks.NewFlags(t)
				m.On("SetOverride", flags.ControlRequestInterval, int64(250*time.Millisecond), 1*time.Second)
				return m
			},
		},
		{
			name:               "no body",
			expectedHttpStatus: http.StatusBadRequest,
			httpErrStr:         "request body is nil",
		},
		{
			name:               "bad interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "blah",
				"duration": "1s",
			},
			httpErrStr: "error parsing interval",
		},
		{
			name:               "empty interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "",
				"duration": "1s",
			},
			httpErrStr: "error parsing interval",
		},
		{
			name:               "no interval",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"duration": "1s",
			},
			httpErrStr: "error parsing interval",
			mockFlags:  func() flags.Flags { return mocks.NewFlags(t) },
		},
		{
			name:               "bad duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "blah",
			},
			httpErrStr: "error parsing duration",
			mockFlags:  func() flags.Flags { return mocks.NewFlags(t) },
		},
		{
			name:               "empty duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
				"duration": "",
			},
			httpErrStr: "error parsing duration",
			mockFlags:  func() flags.Flags { return mocks.NewFlags(t) },
		},
		{
			name:               "no duration",
			expectedHttpStatus: http.StatusBadRequest,
			body: map[string]string{
				"interval": "250ms",
			},
			httpErrStr: "error parsing duration",
			mockFlags:  func() flags.Flags { return mocks.NewFlags(t) },
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var f flags.Flags
			if tt.mockFlags != nil {
				f = tt.mockFlags()
			}

			var logBytes bytes.Buffer
			server := testServer(t, f, &logBytes)

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
