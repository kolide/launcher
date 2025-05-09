package osquerypublisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
)

type osqueryPublisher struct {
	knapsack types.Knapsack
	slogger  *slog.Logger
	client   *http.Client
}

func NewOsqueryPublisher(k types.Knapsack) *osqueryPublisher {
	return &osqueryPublisher{
		slogger: k.Slogger().With("component", "osquery_publisher"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		knapsack: k,
	}
}

type logPayload struct {
	NodeKey string         `json:"node_key"`
	LogType logger.LogType `json:"log_type"`
	Logs    []string       `json:"logs"`
}

type resultPayload struct {
	NodeKey string               `json:"node_key"`
	Results []distributed.Result `json:"results"`
}

func (p *osqueryPublisher) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) error {
	if !p.knapsack.OsqueryPublishEnabled() {
		p.slogger.Log(ctx, slog.LevelDebug,
			"skipping publish logs, not enabled",
		)

		return nil
	}

	payload := logPayload{
		NodeKey: nodeKey,
		LogType: logType,
		Logs:    logs,
	}

	publishUrl := p.knapsack.OsqueryPublishURL()

	if err := p.sendRequest(ctx, publishUrl, payload); err != nil {
		p.slogger.Log(ctx, slog.LevelError,
			"failed to send logs to secondary destination",
			"url", publishUrl,
			"log_type", logType.String(),
			"err", err,
		)
		return fmt.Errorf("sending logs to %s: %w", publishUrl, err)
	}

	p.slogger.Log(ctx, slog.LevelDebug,
		"successfully sent logs to secondary destination",
		"url", publishUrl,
		"log_type", logType.String(),
		"log_count", len(logs),
	)
	return nil
}

func (p *osqueryPublisher) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) error {
	if !p.knapsack.OsqueryPublishEnabled() {
		p.slogger.Log(ctx, slog.LevelDebug,
			"skipping publish results, not enabled",
		)

		return nil
	}

	publishUrl := p.knapsack.OsqueryPublishURL()

	payload := resultPayload{
		NodeKey: nodeKey,
		Results: results,
	}

	if err := p.sendRequest(ctx, publishUrl, payload); err != nil {
		p.slogger.Log(ctx, slog.LevelError,
			"failed to published results",
			"url", publishUrl,
			"err", err,
		)
		return fmt.Errorf("sending results to %s: %w", publishUrl, err)
	}

	p.slogger.Log(ctx, slog.LevelDebug,
		"successfully sent published results",
		"url", publishUrl,
		"result_count", len(results),
	)

	return nil
}

func (p *osqueryPublisher) sendRequest(ctx context.Context, publishUrl string, data any) error {
	if publishUrl == "" {
		return errors.New("publish url is empty")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal json payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, publishUrl, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// TODO: set any auth headers ... deal with auth in general

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-2xx status code: %d %s", resp.StatusCode, resp.Status)
	}

	return nil
}
