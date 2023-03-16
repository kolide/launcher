package localserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log/level"
)

func (ls *localServer) requestAccelerateControlHandler() http.Handler {
	return http.HandlerFunc(ls.requestAccelerateControlFunc)
}

func (ls *localServer) requestAccelerateControlFunc(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		sendClientError(w, "request body is nil")
		return
	}

	if ls.controlService == nil {
		sendClientError(w, fmt.Sprintf("control service not configured"))
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, fmt.Sprintf("error unmarshaling request body: %s", err))
		return
	}

	interval, err := durationFromMap("interval", body)
	if err != nil {
		sendClientError(w, fmt.Sprintf("error parsing interval: %s", err))
		return
	}

	duration, err := durationFromMap("duration", body)
	if err != nil {
		sendClientError(w, fmt.Sprintf("error parsing duration: %s", err))
		return
	}

	if err := ls.controlService.AccelerateRequestInterval(interval, duration); err != nil {
		level.Error(ls.logger).Log(
			"msg", "accelerating control server request interval",
			"err", err,
		)

		sendClientError(w, fmt.Sprintf("error accelerating control server request interval: %s", err))
		return
	}
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
