package localserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kolide/launcher/pkg/backoff"
)

func (ls *localServer) requestQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestQueryHanlderFunc)
}

func (ls *localServer) requestQueryHanlderFunc(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		sendClientError(w, "request body is nil")
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	query, ok := body["query"]
	if !ok || query == "" {
		sendClientError(w, "no query key found in request body json")
		return
	}

	var results []map[string]string
	var err error
	backoff.WaitFor(func() error {
		results, err = ls.querier.Query(query)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	if err != nil {
		sendClientError(w, fmt.Sprintf("error executing query: %s", err))
		return
	}

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		sendClientError(w, fmt.Sprintf("error marshalling results to json: %s", err))
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
	// The driver behind this is that the JS bridge has to use GET requests passing the query (in a nacl box) as a URL parameter.
	// This means there is a limit on the size of the query. This endpoint is intended to be a work around for that. It ought to work like this:
	//
	// 1. K2 looks up the name of a osquery scheduled query it want to be run. The same query should be available to launcher in the osquery_schedule table.
	// 2. K2 calls `/scheduledquery`, with body `{ "name": "name_of_scheduled_query" }`
	// 3. Launcher looks up the query's sql using the provided name
	// 4. Launcher executes the query
	// 5. Launcher returns results

	if r.Body == nil {
		sendClientError(w, "request body is nil")
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	name, ok := body["name"]
	if !ok || name == "" {
		sendClientError(w, "no name key found in request body json")
		return
	}

	scheduledQueryQuery := fmt.Sprintf("select query from osquery_schedule where name = '%s'", name)

	var results []map[string]string
	var err error
	backoff.WaitFor(func() error {
		results, err = ls.querier.Query(scheduledQueryQuery)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	if err != nil {
		sendClientError(w, fmt.Sprintf("error executing query for scheduled using \"%s\": %s", scheduledQueryQuery, err))
		return
	}

	if len(results) == 0 {
		sendClientError(w, fmt.Sprintf("no scheduled query found using \"%s\"", scheduledQueryQuery))
		return
	}

	jsonBytes, err := json.Marshal(results[0])
	if err != nil {
		sendClientError(w, fmt.Sprintf("error marshalling results to json: %s", err))
		return
	}

	// update the request body with the sql from the scheduled query and pass along to query handler
	r.Body = io.NopCloser(bytes.NewBuffer(jsonBytes))

	ls.requestQueryHanlderFunc(w, r)
}

func sendClientError(w http.ResponseWriter, msg string) {
	w.Write([]byte(msg))
	w.WriteHeader(http.StatusBadRequest)
}
