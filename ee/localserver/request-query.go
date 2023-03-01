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

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	queryRaw, ok := body["query"]
	if !ok {
		sendClientError(w, "no query key found in request body json")
		return
	}

	query, ok := queryRaw.(string)
	if !ok {
		sendClientError(w, fmt.Sprintf("query value not a string: %s", query))
		return
	}

	if query == "" {
		sendClientError(w, "empty query")
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
func (ls *localServer) requestRunScheduledQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestRunScheduledQueryHanlderFunc)
}

func (ls *localServer) requestRunScheduledQueryHanlderFunc(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		sendClientError(w, "request body is nil")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	nameRaw, ok := body["name"]
	if !ok {
		sendClientError(w, "no name key found in request body json")
		return
	}

	name, ok := nameRaw.(string)
	if !ok {
		sendClientError(w, fmt.Sprintf("name value not a string: %s", name))
		return
	}

	if name == "" {
		sendClientError(w, "empty name")
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
		sendClientError(w, fmt.Sprintf("error executing query for scheduled query: %s, error: %s", scheduledQueryQuery, err))
		return
	}

	if len(results) == 0 {
		sendClientError(w, fmt.Sprintf("no query found with name '%s' using query %s", name, scheduledQueryQuery))
		return
	}

	query, ok := results[0]["query"]
	if !ok {
		sendClientError(w, fmt.Sprintf("no query found with name '%s' using query \"%s\"", name, scheduledQueryQuery))
		return
	}

	newBody := map[string]string{
		"query": query,
	}

	jsonBytes, err := json.Marshal(newBody)
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
