package localserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/observability"
)

func (ls *localServer) requestAccelerateControlHandler() http.Handler {
	return http.HandlerFunc(ls.requestAccelerateControlFunc)
}

func (ls *localServer) requestAccelerateControlFunc(w http.ResponseWriter, r *http.Request) {
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	if r.Body == nil {
		sendClientError(w, span, errors.New("request body is nil"))
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, span, fmt.Errorf("error unmarshaling request body: %w", err))
		return
	}

	interval, err := durationFromMap("interval", body)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error parsing interval: %w", err))
		return
	}

	duration, err := durationFromMap("duration", body)
	if err != nil {
		sendClientError(w, span, fmt.Errorf("error parsing duration: %w", err))
		return
	}

	// accelerate control server requests
	ls.knapsack.SetControlRequestIntervalOverride(interval, duration)
	// accelerate osquery requests
	ls.knapsack.SetDistributedForwardingIntervalOverride(interval, duration)

	span.AddEvent("control_accelerated")
}

func durationFromMap(key string, body map[string]string) (time.Duration, error) {
	rawDuration, ok := body[key]
	if !ok || rawDuration == "" {
		return 0, fmt.Errorf("no key [%s] found in body", key)
	}

	duration, err := time.ParseDuration(rawDuration)
	if err != nil {
		return 0, fmt.Errorf("error parsing duration [%s]: %w", rawDuration, err)
	}

	return duration, nil
}
