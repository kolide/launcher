package localserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (ls *localServer) requestAccelerateControlHandler() http.Handler {
	return http.HandlerFunc(ls.requestAccelerateControlFunc)
}

func (ls *localServer) requestAccelerateControlFunc(w http.ResponseWriter, r *http.Request) {
	_, span := traces.StartSpan(r.Context(), trace.WithAttributes(attribute.String(traces.AttributeName("localserver", "path"), r.URL.Path)))
	defer span.End()

	if r.Body == nil {
		span.SetStatus(codes.Error, "request body is nil")
		sendClientError(w, "request body is nil")
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	interval, err := durationFromMap("interval", body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		sendClientError(w, fmt.Sprintf("error parsing interval: %s", err))
		return
	}

	duration, err := durationFromMap("duration", body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		sendClientError(w, fmt.Sprintf("error parsing duration: %s", err))
		return
	}

	ls.knapsack.SetControlRequestIntervalOverride(interval, duration)

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
