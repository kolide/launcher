package localserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (ls *localServer) requestControlServerFetchInterval() http.Handler {
	return http.HandlerFunc(ls.requestControlServerFetchIntervalFunc)
}

func (ls *localServer) requestControlServerFetchIntervalFunc(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		sendClientError(w, "request body is nil")
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	rawInterval, ok := body["interval"]
	if !ok || rawInterval == "" {
		sendClientError(w, "no interval key found in request body json")
		return
	}

	interval, err := time.ParseDuration(rawInterval)
	if err != nil {
		sendClientError(w, fmt.Sprintf("error parsing interval, expected format duration:unit (1s, 1m, 1h): %s", err))
		return
	}

	if err := ls.controlServer.UpdateRequestInterval(interval); err != nil {
		sendClientError(w, fmt.Sprintf("error updating fetch interval: %s", err))
		return
	}
}
