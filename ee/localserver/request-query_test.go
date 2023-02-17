package localserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestQueryHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string

		mockQueryResult []map[string]string
		mockQueryError  error

		expectedQueryCount int
		errStr             string
	}{
		{
			name:  "happy path",
			query: "select blah from blah_blah",
			mockQueryResult: []map[string]string{
				{
					"blah": "blah",
				},
			},
			expectedQueryCount: 1,
		},
		{
			name:   "no query",
			errStr: "empty query",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			//go:generate mockery --name Querier
			// https://github.com/vektra/mockery <-- cli tool to generate mocks for usage with testify
			mockQuerier := mocks.NewQuerier(t)
			if tt.expectedQueryCount > 0 {
				mockQuerier.On("Query", tt.query).Return(tt.mockQueryResult, tt.mockQueryError).Times(tt.expectedQueryCount)
			}

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)
			server.querier = mockQuerier

			jsonBytes, err := json.Marshal(map[string]string{
				"query": tt.query,
			})
			req, err := http.NewRequest("", "", bytes.NewBuffer(jsonBytes))
			require.NoError(t, err)

			queryParams := req.URL.Query()
			queryParams.Add("query", tt.query)
			req.URL.RawQuery = queryParams.Encode()

			handler := http.HandlerFunc(server.requestQueryHanlderFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.mockQueryResult != nil {
				require.Equal(t, mustMarshal(t, tt.mockQueryResult), rr.Body.Bytes())
				return
			}

			require.Contains(t, rr.Body.String(), tt.errStr)
		})
	}
}
