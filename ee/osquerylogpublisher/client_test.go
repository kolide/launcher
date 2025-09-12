package osquerylogpublisher

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	osqlog "github.com/osquery/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockHTTPClient struct {
	mock.Mock
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestLogPublisherClient_PublishLogs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		expectErrContains string
		responseBody      string
		responseStatus    int
	}{
		{
			name:              "happy path",
			expectErrContains: "",
			responseBody:      `{"status": "success","ingested_bytes":123,"log_count":1,"message":"Logs ingested successfully"}`,
			responseStatus:    http.StatusOK,
		},
		{
			name:              "non-200 response",
			expectErrContains: "agent-ingester returned status",
			responseBody:      `{"status": "failure"}`,
			responseStatus:    http.StatusUnauthorized,
		},
		{
			name:              "malformed response",
			expectErrContains: "unable to unmarshal agent-ingester response",
			responseBody:      `{"status": "success"...`,
			responseStatus:    http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockKnapsack := mocks.NewKnapsack(t)
			mockHTTPClient := &mockHTTPClient{}
			slogger := multislogger.NewNopLogger()

			mockKnapsack.On("OsqueryLogPublishURL").Return("https://example.com")
			mockKnapsack.On("OsqueryLogPublishAPIKey").Return("test-api-key")

			resp := &http.Response{
				StatusCode: tt.responseStatus,
				Body:       io.NopCloser(bytes.NewReader([]byte(tt.responseBody))),
			}

			mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

			client := &LogPublisherClient{
				knapsack: mockKnapsack,
				client:   mockHTTPClient,
				logger:   slogger,
			}

			logs := []string{"log1", "log2", "log3"}
			result, err := client.PublishLogs(context.Background(), osqlog.LogTypeStatus, logs)

			mockHTTPClient.AssertExpectations(t)

			if tt.expectErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
				return
			}

			// if we expect no error, we expect a properly unmarshalled response with a successful status
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, "success", result.Status)
		})
	}
}
