package menu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kolide/launcher/ee/desktop/runner/server"
	"github.com/kolide/launcher/pkg/authedclient"
)

// Performs the Message action
type actionMessage struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

func (a actionMessage) Perform(m *menu) {
	runnerServerUrl := os.Getenv("RUNNER_SERVER_URL")
	if runnerServerUrl == "" {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"runner server url not set",
		)
		return
	}

	runnerServerAuthToken := os.Getenv("RUNNER_SERVER_AUTH_TOKEN")
	if runnerServerAuthToken == "" {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"runner server auth token not set",
		)
		return
	}

	client := authedclient.New(runnerServerAuthToken, 2*time.Second)
	runnerMessageUrl := fmt.Sprintf("%s%s", runnerServerUrl, server.MessageEndpoint)

	jsonBody, err := json.Marshal(a)
	if err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to marshal message body",
			"err", err,
		)

		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerMessageUrl, bytes.NewReader(jsonBody))
	if err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to create request",
			"err", err,
		)

		return
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
	if err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to perform message action",
			"method", a.Method,
			"params", a.Params,
			"err", err,
		)

		return
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	if response.StatusCode != http.StatusOK {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to perform message action",
			"method", a.Method,
			"params", a.Params,
			"status_code", response.StatusCode,
		)
	}
}
