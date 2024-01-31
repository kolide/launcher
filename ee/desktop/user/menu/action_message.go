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
	runnerHealthUrl := fmt.Sprintf("%s/%s", runnerServerUrl, server.MessageEndpoint)

	body := make(map[string]interface{})
	body["method"] = a.Method
	body["params"] = a.Params

	bodyJson, err := json.Marshal(body)
	if err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to marshal message body",
			"err", err,
		)
		return
	}

	if _, err := client.Post(runnerHealthUrl, "application/json", bytes.NewReader(bodyJson)); err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform message action",
			"method", a.Method,
			"params", a.Params,
			"err", err,
		)
	}
}
