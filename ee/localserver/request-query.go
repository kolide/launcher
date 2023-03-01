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
func (ls *localServer) requestRunScheduledQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestRunScheduledQueryHanlderFunc)
}

func (ls *localServer) requestRunScheduledQueryHanlderFunc(w http.ResponseWriter, r *http.Request) {
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
