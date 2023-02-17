package localserver

import (
	"encoding/json"
	"fmt"
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

func sendClientError(w http.ResponseWriter, msg string) {
	w.Write([]byte(msg))
	w.WriteHeader(http.StatusBadRequest)
}
