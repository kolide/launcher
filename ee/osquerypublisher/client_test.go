package osquerypublisher

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/distributed"
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

			mockKnapsack.On("OsqueryPublisherURL").Return("https://example.com")
			mockKnapsack.On("OsqueryPublisherPercentEnabled").Return(100)
			tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
			tokenStore.Set(storage.AgentIngesterAuthTokenKey, []byte("test-token"))
			require.NoError(t, err)
			mockKnapsack.On("TokenStore").Return(tokenStore).Maybe()

			resp := &http.Response{
				StatusCode: tt.responseStatus,
				Body:       io.NopCloser(bytes.NewReader([]byte(tt.responseBody))),
			}

			mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

			client := NewLogPublisherClient(slogger, mockKnapsack, mockHTTPClient)

			logs := []string{"log1", "log2", "log3"}
			result, err := client.PublishLogs(t.Context(), osqlog.LogTypeStatus, logs)

			mockHTTPClient.AssertExpectations(t)

			if tt.expectErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				// if we expect no error, we expect a properly unmarshalled response with a successful status
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "success", result.Status)
			}
		})
	}
}

func TestLogPublisherClient_PublishResults(t *testing.T) {
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
			responseBody:      `{"status": "success","ingested_bytes":123,"log_count":1,"message":"Results ingested successfully"}`,
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

			mockKnapsack.On("OsqueryPublisherURL").Return("https://example.com")
			mockKnapsack.On("OsqueryPublisherPercentEnabled").Return(100)
			tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
			tokenStore.Set(storage.AgentIngesterAuthTokenKey, []byte("test-token"))
			require.NoError(t, err)
			mockKnapsack.On("TokenStore").Return(tokenStore).Maybe()

			resp := &http.Response{
				StatusCode: tt.responseStatus,
				Body:       io.NopCloser(bytes.NewReader([]byte(tt.responseBody))),
			}

			mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

			client := NewLogPublisherClient(slogger, mockKnapsack, mockHTTPClient)

			results := []distributed.Result{
				{
					QueryName: "test_query",
					Status:    0,
					Rows: []map[string]string{
						{
							"column1": "value1",
							"column2": "value2",
						},
					},
				},
			}
			result, err := client.PublishResults(t.Context(), results)

			mockHTTPClient.AssertExpectations(t)

			if tt.expectErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				// if we expect no error, we expect a properly unmarshalled response with a successful status
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "success", result.Status)
			}
		})
	}
}

func TestLogPublisherClient_shouldPublishLogs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		percentEnabled    int
		url               string
		shouldPublishLogs bool
	}{
		{
			name:              "default disabled",
			percentEnabled:    0,
			url:               "https://example.com",
			shouldPublishLogs: false,
		},
		{
			name:              "full cutover enabled",
			percentEnabled:    100,
			url:               "https://example.com",
			shouldPublishLogs: true,
		},
		{
			name:              "full cutover enabled without url",
			percentEnabled:    100,
			url:               "",
			shouldPublishLogs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockKnapsack := mocks.NewKnapsack(t)
			mockHTTPClient := &mockHTTPClient{}
			slogger := multislogger.NewNopLogger()

			// these mocks are all set to maybe because the order of what is set will impact what is actually checked
			mockKnapsack.On("OsqueryPublisherPercentEnabled").Return(tt.percentEnabled).Maybe()
			mockKnapsack.On("OsqueryPublisherURL").Return(tt.url).Maybe()
			client := &LogPublisherClient{
				slogger:  slogger.With("component", "osquery_log_publisher"),
				knapsack: mockKnapsack,
				tokens:   make(map[string]string),
			}

			assert.Equal(t, tt.shouldPublishLogs, client.shouldPublishLogs())
			mockHTTPClient.AssertExpectations(t)
		})
	}
}

func TestLogPublisherClient_BatchLogsRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		logs            []string
		expectedBatches [][]string
	}{
		{
			name:            "empty logs",
			logs:            []string{},
			expectedBatches: [][]string{},
		},
		{
			name:            "single small log",
			logs:            []string{"small log"},
			expectedBatches: [][]string{{"small log"}},
		},
		{
			name:            "multiple small logs in one batch",
			logs:            []string{"log1", "log2", "log3"},
			expectedBatches: [][]string{{"log1", "log2", "log3"}},
		},
		{
			name: "logs that need to be split into two batches",
			logs: []string{
				string(make([]byte, maxRequestSizeBytes-1)), // log that's just under the limit
				"small log", // this will start a new batch
			},
			expectedBatches: [][]string{
				{string(make([]byte, maxRequestSizeBytes-1))},
				{"small log"},
			},
		},
		{
			name: "logs that need to be split into multiple batches",
			logs: []string{
				string(make([]byte, maxRequestSizeBytes/2)), // half the max size
				string(make([]byte, maxRequestSizeBytes/2)), // half the max size (fits in same batch)
				string(make([]byte, maxRequestSizeBytes/2)), // half the max size (starts new batch)
				"small log", // fits in second batch
			},
			expectedBatches: [][]string{
				{
					string(make([]byte, maxRequestSizeBytes/2)),
					string(make([]byte, maxRequestSizeBytes/2)),
				},
				{
					string(make([]byte, maxRequestSizeBytes/2)),
					"small log",
				},
			},
		},
		{
			name: "single log exactly at max size",
			logs: []string{
				string(make([]byte, maxRequestSizeBytes)),
			},
			expectedBatches: [][]string{
				{string(make([]byte, maxRequestSizeBytes))},
			},
		},
		{
			name: "single log exceeding max size",
			logs: []string{
				string(make([]byte, maxRequestSizeBytes+1)),
			},
			expectedBatches: [][]string{
				{string(make([]byte, maxRequestSizeBytes+1))},
			},
		},
		{
			name: "multiple batches with single log exceeding max size",
			logs: []string{
				"small first log",
				string(make([]byte, maxRequestSizeBytes-100)),
				string(make([]byte, maxRequestSizeBytes+1)),
				"small last log",
			},
			expectedBatches: [][]string{
				{ // note that the single log will get put in its own batch before completing the remaining logs in the original batch
					string(make([]byte, maxRequestSizeBytes+1)),
				},
				{
					"small first log",
					string(make([]byte, maxRequestSizeBytes-100)),
					"small last log",
				},
			},
		},
		{
			name: "logs that exactly fill a batch",
			logs: []string{
				string(make([]byte, maxRequestSizeBytes/2)),
				string(make([]byte, maxRequestSizeBytes/2)),
			},
			expectedBatches: [][]string{
				{
					string(make([]byte, maxRequestSizeBytes/2)),
					string(make([]byte, maxRequestSizeBytes/2)),
				},
			},
		},
		{
			name: "many small logs that all fit in one batch",
			logs: []string{
				"log1", "log2", "log3", "log4", "log5",
			},
			expectedBatches: [][]string{
				{"log1", "log2", "log3", "log4", "log5"},
			},
		},
		{
			name: "logs with varying sizes",
			logs: []string{
				string(make([]byte, 50)),
				string(make([]byte, 100)),
				string(make([]byte, 150)),
				string(make([]byte, maxRequestSizeBytes-301)),
				"another small one that should be batched separately",
			},
			expectedBatches: [][]string{
				{
					string(make([]byte, 50)),
					string(make([]byte, 100)),
					string(make([]byte, 150)),
					string(make([]byte, maxRequestSizeBytes-301)),
				},
				{"another small one that should be batched separately"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			batches := BatchLogsRequest(multislogger.NewNopLogger(), tt.logs)

			require.Equal(t, len(tt.expectedBatches), len(batches), "number of batches should match")

			for i, expectedBatch := range tt.expectedBatches {
				require.Equal(t, len(expectedBatch), len(batches[i]), "batch %d should have correct number of logs", i)
				require.Equal(t, expectedBatch, batches[i], "batch %d should match expected logs", i)
			}

			// now check each batch- if any exceeds the maxRequestSize, verify that it is a solo entry (batch of size 1).
			// otherwise, verify that the total log length does not exceed our limit
			for _, logs := range batches {
				totalBatchSize := 0
				for _, log := range logs {
					if len(log) > maxRequestSizeBytes {
						require.Equal(t, 1, len(logs))
					} else {
						totalBatchSize += len(log)
						// verify that the total size never exceeds limit
						require.LessOrEqual(t, totalBatchSize, maxRequestSizeBytes)
					}
				}
			}
		})
	}
}
