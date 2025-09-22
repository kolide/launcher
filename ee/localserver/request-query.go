package localserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/osquery/osquery-go/plugin/distributed"
	"go.opentelemetry.io/otel/trace"
)

func (ls *localServer) requestQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestQueryHanlderFunc)
}

func (ls *localServer) requestQueryHanlderFunc(w http.ResponseWriter, r *http.Request) {
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	if r.Body == nil {
		sendClientError(w, span, errors.New("request body is nil"))
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, span, fmt.Errorf("error unmarshaling request body: %s", err))
		return
	}

	query, ok := body["query"]
	if !ok || query == "" {
		sendClientError(w, span, errors.New("no query key found in request body json"))
		return
	}

	results, err := queryWithRetries(ls.querier, query)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error executing query: %s", err))
		return
	}

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error marshalling results to json: %s", err))
		return
	}

	w.Write(jsonBytes)
}

func (ls *localServer) requestScheduledQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestScheduledQueryHandlerFunc)
}

// requestScheduledQueryHandlerFunc uses the name field in the request body to look up
// an existing osquery scheduled query execute it, returning the results.
func (ls *localServer) requestScheduledQueryHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	// The driver behind this is that the JS bridge has to use GET requests passing the query (in a nacl box) as a URL parameter.
	// This means there is a limit on the size of the query. This endpoint is intended to be a work around for that. It ought to work like this:
	//
	// 1. K2 looks up the name of a osquery scheduled query it want to be run. The same query should be available to launcher in the osquery_schedule table.
	// 2. K2 calls `/scheduledquery`, with body `{ "name": "name_of_scheduled_query" }`
	// 3. Launcher looks up the query's sql using the provided name
	// 4. Launcher executes the query
	// 5. Launcher returns results

	if r.Body == nil {
		sendClientError(w, span, errors.New("request body is nil"))
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, span, fmt.Errorf("error unmarshaling request body: %s", err))
		return
	}

	name, ok := body["name"]
	if !ok || name == "" {
		sendClientError(w, span, errors.New("no name key found in request body json"))
		return
	}

	scheduledQueryQuery := fmt.Sprintf("select name, query from osquery_schedule where name like '%s'", name)

	scheduledQueriesQueryResults, err := queryWithRetries(ls.querier, scheduledQueryQuery)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error executing query for scheduled queries using \"%s\": %s", scheduledQueryQuery, err))
		return
	}

	results := make([]distributed.Result, len(scheduledQueriesQueryResults))

	for i, scheduledQuery := range scheduledQueriesQueryResults {
		results[i] = distributed.Result{
			QueryName: scheduledQuery["name"],
		}

		scheduledQueryResult, err := queryWithRetries(ls.querier, scheduledQuery["query"])
		if err != nil {
			ls.slogger.Log(r.Context(), slog.LevelError,
				"running scheduled query on demand",
				"err", err,
				"query", scheduledQuery["query"],
				"query_name", scheduledQuery["name"],
			)

			results[i].Status = 1
			continue
		}

		// no error
		results[i].Rows = scheduledQueryResult
	}

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error marshalling results to json: %w", err))
		return
	}

	w.Write(jsonBytes)
}

func sendClientError(w http.ResponseWriter, span trace.Span, err error) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(err.Error()))

	observability.SetError(span, err)
}

func queryWithRetries(querier Querier, query string) ([]map[string]string, error) {
	var results []map[string]string
	var err error

	backoff.WaitFor(func() error {
		results, err = querier.Query(query)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	return results, err
}
