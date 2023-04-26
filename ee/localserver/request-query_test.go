package localserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	typesMocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/osquery/osquery-go/plugin/distributed"
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
			errStr: "no query key found in request body json",
		},
		{
			name:   "empty query",
			query:  "",
			errStr: "no query key found in request body json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("ConfigStore").Return(storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String()))
			mockKnapsack.On("KolideServerURL").Return("localhost")

			//go:generate mockery --name Querier
			// https://github.com/vektra/mockery <-- cli tool to generate mocks for usage with testify
			mockQuerier := mocks.NewQuerier(t)

			if tt.mockQueryResult != nil {
				mockQuerier.On("Query", tt.query).Return(tt.mockQueryResult, nil).Once()
			}

			var logBytes bytes.Buffer
			server := testServer(t, mockKnapsack, &logBytes)
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

	type queryReturn struct {
		results []map[string]string
		err     error
	}

	test := []struct {
		testName                         string
		scheduledQueriesQueryNamePattern string
		scheduledQueriesQueryResults     []map[string]string
		scheduledQueriesQueryError       error
		queryReturns                     []queryReturn
		expectedResult                   []distributed.Result
	}{
		{
			testName:                         "no scheduled queries",
			scheduledQueriesQueryNamePattern: "%does_not_exist%",
			scheduledQueriesQueryResults:     []map[string]string{},
			expectedResult:                   []distributed.Result{},
		},
		{
			testName:                         "one scheduled query",
			scheduledQueriesQueryNamePattern: "%one_query_found%",
			scheduledQueriesQueryResults: []map[string]string{
				{
					"query": "select * from one",
					"name":  "one",
				},
			},
			queryReturns: []queryReturn{
				{
					results: []map[string]string{
						{
							"one": "one",
						},
					},
				},
			},
			expectedResult: []distributed.Result{
				{
					QueryName: "one",
					Status:    0,
					Rows: []map[string]string{
						{
							"one": "one",
						},
					},
				},
			},
		},
		{
			testName:                         "multiple scheduled queries with error",
			scheduledQueriesQueryNamePattern: "%three_queries_found%",
			scheduledQueriesQueryResults: []map[string]string{
				{
					"query": "select * from one",
					"name":  "one",
				},
				{
					"query": "select * from two",
					"name":  "two",
				},
				{
					"query": "select * from three",
					"name":  "three",
				},
			},
			queryReturns: []queryReturn{
				{
					results: []map[string]string{
						{
							"one": "one",
						},
					},
				},
				{
					err: errors.New("error two"),
				},
				{
					results: []map[string]string{
						{
							"three": "three",
						},
					},
				},
			},
			expectedResult: []distributed.Result{
				{
					QueryName: "one",
					Status:    0,
					Rows: []map[string]string{
						{
							"one": "one",
						},
					},
				},
				{
					QueryName: "two",
					Status:    1,
				},
				{
					QueryName: "three",
					Status:    0,
					Rows: []map[string]string{
						{
							"three": "three",
						},
					},
				},
			},
		},
		{
			testName:                         "scheduled queries query error",
			scheduledQueriesQueryNamePattern: "%scheduled_queries_query_error%",
			scheduledQueriesQueryError:       errors.New("scheduled query error"),
		},
	}

	for _, tt := range test {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("ConfigStore").Return(storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String()))
			mockKnapsack.On("KolideServerURL").Return("localhost")

			// set up mock querier
			mockQuerier := mocks.NewQuerier(t)
			scheduledQueryQuery := fmt.Sprintf("select name, query from osquery_schedule where name like '%s'", tt.scheduledQueriesQueryNamePattern)

			// the query for the scheduled queries
			mockQuerier.On("Query", scheduledQueryQuery).Return(tt.scheduledQueriesQueryResults, tt.scheduledQueriesQueryError)

			// the results of each scheduled query
			for i, queryResult := range tt.queryReturns {
				mockQuerier.On("Query", tt.scheduledQueriesQueryResults[i]["query"]).Return(queryResult.results, queryResult.err)
			}

			// set up test server
			var logBytes bytes.Buffer
			server := testServer(t, mockKnapsack, &logBytes)
			server.querier = mockQuerier

			// make request body
			body := make(map[string]string)
			body["name"] = tt.scheduledQueriesQueryNamePattern
			jsonBytes, err := json.Marshal(body)
			require.NoError(t, err)

			// set up request
			req, err := http.NewRequest("", "", bytes.NewBuffer(jsonBytes))
			require.NoError(t, err)

			// set up handler
			handler := http.HandlerFunc(server.requestScheduledQueryHandlerFunc)
			rr := httptest.NewRecorder()

			// server request
			handler.ServeHTTP(rr, req)

			if tt.scheduledQueriesQueryError != nil {
				require.Contains(t, rr.Body.String(), tt.scheduledQueriesQueryError.Error())
				return
			}

			require.Equal(t, mustMarshal(t, tt.expectedResult), rr.Body.Bytes())
			return
		})
	}
}
