package localserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/ee/presencedetection"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPresenceDetectionHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                              string
		expectDetectPresenceCall                          bool
		intervalHeader, reasonHeader                      string
		durationSinceLastDetection                        time.Duration
		presenceDetectionError                            error
		shouldHavePresenceDetectionDurationResponseHeader bool
		expectedStatusCode                                int
	}{
		{
			name:               "no presence detection headers",
			expectedStatusCode: http.StatusOK,
			shouldHavePresenceDetectionDurationResponseHeader: false,
		},
		{
			name:               "invalid presence detection interval",
			intervalHeader:     "invalid-interval",
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:                       "valid presence detection, detection fails",
			expectDetectPresenceCall:   true,
			intervalHeader:             "10s",
			reasonHeader:               "test reason",
			durationSinceLastDetection: presencedetection.DetectionFailedDurationValue,
			expectedStatusCode:         http.StatusOK,
			shouldHavePresenceDetectionDurationResponseHeader: true,
		},
		{
			name:                       "valid presence detection, detection succeeds",
			expectDetectPresenceCall:   true,
			intervalHeader:             "10s",
			reasonHeader:               "test reason",
			durationSinceLastDetection: 0,
			expectedStatusCode:         http.StatusOK,
			shouldHavePresenceDetectionDurationResponseHeader: true,
		},
		{
			name:                       "presence detection error",
			expectDetectPresenceCall:   true,
			intervalHeader:             "10s",
			reasonHeader:               "test reason",
			durationSinceLastDetection: presencedetection.DetectionFailedDurationValue,
			presenceDetectionError:     assert.AnError,
			expectedStatusCode:         http.StatusOK,
			shouldHavePresenceDetectionDurationResponseHeader: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockPresenceDetector := mocks.NewPresenceDetector(t)

			if tt.expectDetectPresenceCall {
				mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).Return(tt.durationSinceLastDetection, tt.presenceDetectionError)
			}

			server := &localServer{
				presenceDetector: mockPresenceDetector,
				slogger:          multislogger.NewNopLogger(),
			}

			// Create a test handler for the middleware to call
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap the test handler in the middleware
			handlerToTest := server.presenceDetectionHandler(nextHandler)

			// Create a request with the specified headers
			req := httptest.NewRequest("GET", "/", nil)
			if tt.intervalHeader != "" {
				req.Header.Add("X-Kolide-Presence-Detection-Interval", tt.intervalHeader)
			}

			if tt.reasonHeader != "" {
				req.Header.Add("X-Kolide-Presence-Detection-Reason", tt.reasonHeader)
			}

			rr := httptest.NewRecorder()
			handlerToTest.ServeHTTP(rr, req)

			if tt.shouldHavePresenceDetectionDurationResponseHeader {
				require.NotEmpty(t, rr.Header().Get(kolideDurationSinceLastPresenceDetection))
			}
			require.Equal(t, tt.expectedStatusCode, rr.Code)
		})
	}
}
