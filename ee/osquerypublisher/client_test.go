package osquerypublisher

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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

// makeTestLogStringOfEncodedLength is a helper function to make a string that will be
// a specific length after encoding. this is required because making e.g. a null byte array
// will change in length dramatically as it is encoded
func makeLogStringOfEncodedLength(length int) string {
	// subtract 2 because the string will be encoded as a json string, which will add 2 quotes
	return strings.Repeat("a", length-2)
}

func TestLogPublisherClient_batchLogsRequest(t *testing.T) {
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
				makeLogStringOfEncodedLength(maxRequestSizeBytes - 1), // log that's just under the limit
				"small log", // this will start a new batch
			},
			expectedBatches: [][]string{
				{makeLogStringOfEncodedLength(maxRequestSizeBytes - 1)},
				{"small log"},
			},
		},
		{
			name: "logs that need to be split into multiple batches",
			logs: []string{
				makeLogStringOfEncodedLength(maxRequestSizeBytes / 2), // half the max size
				makeLogStringOfEncodedLength(maxRequestSizeBytes / 2), // half the max size (fits in same batch)
				makeLogStringOfEncodedLength(maxRequestSizeBytes / 2), // half the max size (starts new batch)
				"small log", // fits in second batch
			},
			expectedBatches: [][]string{
				{
					makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
					makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
				},
				{
					makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
					"small log",
				},
			},
		},
		{
			name: "single log exactly at max size",
			logs: []string{
				makeLogStringOfEncodedLength(maxRequestSizeBytes),
			},
			expectedBatches: [][]string{
				{makeLogStringOfEncodedLength(maxRequestSizeBytes)},
			},
		},
		{
			name: "single log exceeding max size",
			logs: []string{
				makeLogStringOfEncodedLength(maxRequestSizeBytes + 1),
			},
			expectedBatches: [][]string{
				{makeLogStringOfEncodedLength(maxRequestSizeBytes + 1)},
			},
		},
		{
			name: "multiple batches with single log exceeding max size",
			logs: []string{
				"small first log",
				makeLogStringOfEncodedLength(maxRequestSizeBytes - 100),
				makeLogStringOfEncodedLength(maxRequestSizeBytes + 1),
				"small last log",
			},
			expectedBatches: [][]string{
				{ // note that the single log will get put in its own batch before completing the remaining logs in the original batch
					makeLogStringOfEncodedLength(maxRequestSizeBytes + 1),
				},
				{
					"small first log",
					makeLogStringOfEncodedLength(maxRequestSizeBytes - 100),
					"small last log",
				},
			},
		},
		{
			name: "logs that exactly fill a batch",
			logs: []string{
				makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
				makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
			},
			expectedBatches: [][]string{
				{
					makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
					makeLogStringOfEncodedLength(maxRequestSizeBytes / 2),
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
				makeLogStringOfEncodedLength(50),
				makeLogStringOfEncodedLength(100),
				makeLogStringOfEncodedLength(150),
				makeLogStringOfEncodedLength(maxRequestSizeBytes - 301),
				"another small one that should be batched separately",
			},
			expectedBatches: [][]string{
				{
					makeLogStringOfEncodedLength(50),
					makeLogStringOfEncodedLength(100),
					makeLogStringOfEncodedLength(150),
					makeLogStringOfEncodedLength(maxRequestSizeBytes - 301),
				},
				{"another small one that should be batched separately"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			batches := batchRequest(tt.logs, multislogger.NewNopLogger())

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

// makeResultOfEncodedLength is a helper function to make a distributed.Result that will be
// a specific length after JSON encoding. This accounts for the JSON structure overhead.
// don't attempt to make a result smaller than the base overhead (76 bytes), the best we can
// do there is give back a 76 byte empty result but that seems more confusing than just failing
func makeResultOfEncodedLength(t *testing.T, length int) distributed.Result {
	t.Helper()
	// measure the overhead by creating a test result with empty data
	testRow := map[string]string{"data": ""}
	testResult := distributed.Result{
		QueryName: "t",
		Status:    0,
		Rows:      []map[string]string{testRow},
	}
	testJSON, _ := json.Marshal(testResult)
	baseOverhead := len(testJSON)

	// calculate the actual data size needed
	dataSize := length - baseOverhead
	// don't want to constantly error check for a helper function but the results are
	// confusing if you try to target a length that's smaller than the base overhead
	// (e.g. if try to make a result of 50 bytes) so fail loudly
	require.Greater(t, dataSize, 0, "base overhead is 76 bytes, update your test to use a bigger target length")

	// Create the result with the calculated data size
	row := map[string]string{
		"data": strings.Repeat("a", dataSize),
	}

	testResult.Rows = []map[string]string{row}

	return testResult
}

func TestLogPublisherClient_batchResultsRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		results         []distributed.Result
		expectedBatches [][]distributed.Result
	}{
		{
			name:            "empty results",
			results:         []distributed.Result{},
			expectedBatches: [][]distributed.Result{},
		},
		{
			name: "single small result",
			results: []distributed.Result{
				{QueryName: "test_query", Status: 0, Rows: []map[string]string{{"key": "value"}}},
			},
			expectedBatches: [][]distributed.Result{
				{{QueryName: "test_query", Status: 0, Rows: []map[string]string{{"key": "value"}}}},
			},
		},
		{
			name: "multiple small results in one batch",
			results: []distributed.Result{
				{QueryName: "query1", Status: 0, Rows: []map[string]string{{"key1": "value1"}}},
				{QueryName: "query2", Status: 0, Rows: []map[string]string{{"key2": "value2"}}},
				{QueryName: "query3", Status: 0, Rows: []map[string]string{{"key3": "value3"}}},
			},
			expectedBatches: [][]distributed.Result{
				{
					{QueryName: "query1", Status: 0, Rows: []map[string]string{{"key1": "value1"}}},
					{QueryName: "query2", Status: 0, Rows: []map[string]string{{"key2": "value2"}}},
					{QueryName: "query3", Status: 0, Rows: []map[string]string{{"key3": "value3"}}},
				},
			},
		},
		{
			name: "results that need to be split into two batches",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, maxRequestSizeBytes-1),                                // result that's just under the limit
				{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}}, // this will start a new batch
			},
			expectedBatches: [][]distributed.Result{
				{makeResultOfEncodedLength(t, maxRequestSizeBytes-1)},
				{{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}}},
			},
		},
		{
			name: "results that need to be split into multiple batches",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, maxRequestSizeBytes/2),                                // half the max size
				makeResultOfEncodedLength(t, maxRequestSizeBytes/2),                                // half the max size (fits in same batch)
				makeResultOfEncodedLength(t, maxRequestSizeBytes/2),                                // half the max size (starts new batch)
				{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}}, // fits in second batch
			},
			expectedBatches: [][]distributed.Result{
				{
					makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
					makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
				},
				{
					makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
					{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}},
				},
			},
		},
		{
			name: "single result exactly at max size",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, maxRequestSizeBytes),
			},
			expectedBatches: [][]distributed.Result{
				{makeResultOfEncodedLength(t, maxRequestSizeBytes)},
			},
		},
		{
			name: "single result exceeding max size",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, maxRequestSizeBytes+1),
			},
			expectedBatches: [][]distributed.Result{
				{makeResultOfEncodedLength(t, maxRequestSizeBytes+1)},
			},
		},
		{
			name: "multiple batches with single result exceeding max size",
			results: []distributed.Result{
				{QueryName: "small_first", Status: 0, Rows: []map[string]string{{"key": "value"}}},
				makeResultOfEncodedLength(t, maxRequestSizeBytes-300),
				makeResultOfEncodedLength(t, maxRequestSizeBytes+1),
				{QueryName: "small_last", Status: 0, Rows: []map[string]string{{"key": "value"}}},
			},
			expectedBatches: [][]distributed.Result{
				{
					makeResultOfEncodedLength(t, maxRequestSizeBytes+1),
				},
				{
					{QueryName: "small_first", Status: 0, Rows: []map[string]string{{"key": "value"}}},
					makeResultOfEncodedLength(t, maxRequestSizeBytes-300),
					{QueryName: "small_last", Status: 0, Rows: []map[string]string{{"key": "value"}}},
				},
			},
		},
		{
			name: "results that almost fill a batch",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
				makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
			},
			expectedBatches: [][]distributed.Result{
				{
					makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
					makeResultOfEncodedLength(t, maxRequestSizeBytes/2),
				},
			},
		},
		{
			name: "many small results that all fit in one batch",
			results: []distributed.Result{
				{QueryName: "query1", Status: 0, Rows: []map[string]string{{"key1": "value1"}}},
				{QueryName: "query2", Status: 0, Rows: []map[string]string{{"key2": "value2"}}},
				{QueryName: "query3", Status: 0, Rows: []map[string]string{{"key3": "value3"}}},
				{QueryName: "query4", Status: 0, Rows: []map[string]string{{"key4": "value4"}}},
				{QueryName: "query5", Status: 0, Rows: []map[string]string{{"key5": "value5"}}},
			},
			expectedBatches: [][]distributed.Result{
				{
					{QueryName: "query1", Status: 0, Rows: []map[string]string{{"key1": "value1"}}},
					{QueryName: "query2", Status: 0, Rows: []map[string]string{{"key2": "value2"}}},
					{QueryName: "query3", Status: 0, Rows: []map[string]string{{"key3": "value3"}}},
					{QueryName: "query4", Status: 0, Rows: []map[string]string{{"key4": "value4"}}},
					{QueryName: "query5", Status: 0, Rows: []map[string]string{{"key5": "value5"}}},
				},
			},
		},
		{
			name: "results with varying sizes",
			results: []distributed.Result{
				makeResultOfEncodedLength(t, 100),
				makeResultOfEncodedLength(t, 100),
				makeResultOfEncodedLength(t, 100),
				makeResultOfEncodedLength(t, maxRequestSizeBytes-301),
				{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}},
			},
			expectedBatches: [][]distributed.Result{
				{
					makeResultOfEncodedLength(t, 100),
					makeResultOfEncodedLength(t, 100),
					makeResultOfEncodedLength(t, 100),
					makeResultOfEncodedLength(t, maxRequestSizeBytes-301),
				},
				{
					{QueryName: "small_query", Status: 0, Rows: []map[string]string{{"key": "value"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			batches := batchRequest(tt.results, multislogger.NewNopLogger())

			require.Equal(t, len(tt.expectedBatches), len(batches), "number of batches should match")

			for i, expectedBatch := range tt.expectedBatches {
				require.Equal(t, len(expectedBatch), len(batches[i]), "batch %d should have correct number of results", i)
				expectedBatchRaw, err := json.Marshal(expectedBatch)
				require.NoError(t, err)
				batchRaw, err := json.Marshal(batches[i])
				require.NoError(t, err)
				require.Equal(t, expectedBatchRaw, batchRaw, "batch %d should match expected results", i)
			}

			// now check each batch- if any exceeds the maxRequestSize, verify that it is a solo entry (batch of size 1).
			// otherwise, verify that the total result length does not exceed our limit
			for _, results := range batches {
				totalBatchSize := 0
				for _, result := range results {
					resultJSON, _ := json.Marshal(result)
					resultLen := len(resultJSON)
					if resultLen > maxRequestSizeBytes {
						require.Equal(t, 1, len(results), "results exceeding max size should be in their own batch")
					} else {
						totalBatchSize += resultLen
						// verify that the total size never exceeds limit
						require.LessOrEqual(t, totalBatchSize, maxRequestSizeBytes, "batch total size should not exceed max")
					}
				}
			}
		})
	}
}
