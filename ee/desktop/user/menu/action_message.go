package menu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log/level"
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
		level.Error(m.logger).Log(
			"msg", "runner server url not set",
		)
		return
	}

	runnerServerAuthToken := os.Getenv("RUNNER_SERVER_AUTH_TOKEN")
	if runnerServerAuthToken == "" {
		level.Error(m.logger).Log(
			"msg", "runner server auth token not set",
		)
		return
	}

	client := authedclient.New(runnerServerAuthToken, 2*time.Second)
	runnerMessageUrl := fmt.Sprintf("%s%s", runnerServerUrl, server.MessageEndpoint)

	jsonBody, err := json.Marshal(a)
	if err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to marshal message body",
			"err", err,
		)
		return
	}

	response, err := client.Post(runnerMessageUrl, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform message action",
			"method", a.Method,
			"params", a.Params,
			"err", err,
		)

		return
	}

	if response.StatusCode != 200 {
		level.Error(m.logger).Log(
			"msg", "failed to perform message action",
			"method", a.Method,
			"params", a.Params,
			"status_code", response.StatusCode,
		)
	}
}
