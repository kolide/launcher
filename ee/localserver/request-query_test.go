package localserver

import (
	"bytes"
	"encoding/json"
	"fmt"
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

		errStr string
	}{
		{
			name:  "happy path",
			query: "select blah from blah_blah",
			mockQueryResult: []map[string]string{
				{
					"blah": "blah",
				},
			},
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

			if tt.mockQueryResult != nil {
				mockQuerier.On("Query", tt.query).Return(tt.mockQueryResult, nil).Once()
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

func Test_localServer_requestRunScheduledQueryHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName  string
		queryName string

		// a successful test would have 2 queries

		// mockScheduledQuerySqlFetchResults - the first one returns the sql from the scheduled query
		mockScheduledQuerySqlFetchResults []map[string]string

		// mockQueryResults -the second one returns the results of the sql fetched in previous query
		mockQueryResults []map[string]string

		errStr string
	}{
		{
			testName:  "happy path",
			queryName: "some_scheduled_query",
			mockScheduledQuerySqlFetchResults: []map[string]string{
				{
					"query": "select * from some_table",
				},
			},
			mockQueryResults: []map[string]string{
				{
					"results (could be anything)": "results of query",
				},
			},
		},
		{
			testName:  "no results",
			queryName: "some_scheduled_query",
			mockScheduledQuerySqlFetchResults: []map[string]string{
				{},
			},
			errStr: "no query found",
		},
		{
			testName:  "no query row",
			queryName: "some_scheduled_query",
			mockScheduledQuerySqlFetchResults: []map[string]string{
				{
					"foo": "bar",
				},
			},
			errStr: "no query found",
		},
		{
			testName: "no query name",
			errStr:   "no name key found in request body",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			//go:generate mockery --name Querier
			// https://github.com/vektra/mockery <-- cli tool to generate mocks for usage with testify
			mockQuerier := mocks.NewQuerier(t)

			if tt.mockScheduledQuerySqlFetchResults != nil {
				// first query to get the sql of the scheduled query
				mockQuerier.On("Query", fmt.Sprintf("select query from osquery_schedule where name = '%s'", tt.queryName)).Return(tt.mockScheduledQuerySqlFetchResults, nil).Once()
			}

			if tt.mockQueryResults != nil {
				// second query to get the results of the sql fetched in previous query
				mockQuerier.On("Query", tt.mockScheduledQuerySqlFetchResults[0]["query"]).Return(tt.mockQueryResults, nil).Once()
			}

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)
			server.querier = mockQuerier

			body := make(map[string]string)

			if tt.queryName != "" {
				body["name"] = tt.queryName
			}

			jsonBytes, err := json.Marshal(body)
			require.NoError(t, err)

			req, err := http.NewRequest("", "", bytes.NewBuffer(jsonBytes))
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestRunScheduledQueryHanlderFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.errStr != "" {
				require.Contains(t, rr.Body.String(), tt.errStr)
				return
			}

			require.Equal(t, mustMarshal(t, tt.mockQueryResults), rr.Body.Bytes())
			return
		})
	}
}
