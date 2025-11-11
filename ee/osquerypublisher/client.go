package osquerypublisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/kolide/kit/contexts/uuid"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/service"
	osqlog "github.com/osquery/osquery-go/plugin/logger"
)

type (
	// LogPublisherClient adheres to the Publisher interface. It handles log publication
	// to the agent-ingester microservice
	LogPublisherClient struct {
		logger   *slog.Logger
		knapsack types.Flags
		client   PublisherHTTPClient
	}
)

func NewLogPublisherClient(logger *slog.Logger, k types.Flags, client PublisherHTTPClient) Publisher {
	return &LogPublisherClient{
		logger:   logger.With("component", "osquery_log_publisher"),
		knapsack: k,
		client:   client,
	}
}

// helper method to allow us to make any http client tweaks as we learn realistic
// parameters for interacting with the agent-ingester service
func NewPublisherHTTPClient() PublisherHTTPClient {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

func (lpc *LogPublisherClient) PublishLogs(ctx context.Context, logType osqlog.LogType, logs []string) (*PublishLogsResponse, error) {
	if !lpc.shouldPublishLogs() {
		return nil, nil
	}

	requestUUID := uuid.NewForRequest()
	ctx = uuid.NewContext(ctx, requestUUID)
	logger := lpc.logger.With(
		"request_uuid", requestUUID,
		"log_type", logType.String(),
		"log_count", len(logs),
	)
	var resp *http.Response
	var publishLogsResponse PublishLogsResponse
	var err error

	defer func(begin time.Time) {
		pubStateVals, ok := ctx.Value(service.PublicationCtxKey).(map[string]int)
		if !ok {
			pubStateVals = make(map[string]int)
		}

		logger.Log(ctx, levelForError(err), "attempted log publication",
			"method", "PublishLogs",
			"response", publishLogsResponse,
			"status_code", resp.StatusCode,
			"err", err,
			"took", time.Since(begin),
			"publication_state", pubStateVals,
		)
	}(time.Now())

	payload := PublishLogsRequest{
		LogType: logType,
		Logs:    logs,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to marshal log publish request",
			"err", err,
		)
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/logs", lpc.knapsack.OsqueryLogPublishURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to create HTTP request",
			"err", err,
		)
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// set required headers and issue the request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lpc.knapsack.OsqueryLogPublishAPIKey()))
	resp, err = lpc.client.Do(req)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to issue HTTP request",
			"err", err,
		)
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// read in the response and unmarshal
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to read response body",
			"status_code", resp.StatusCode,
			"err", err,
		)
		return nil, fmt.Errorf("reading response: %w", err)
	}

	err = json.Unmarshal(body, &publishLogsResponse)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to unmarshal response body",
			"status_code", resp.StatusCode,
			"err", err,
		)
		return nil, fmt.Errorf("unable to unmarshal agent-ingester response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Log(ctx, slog.LevelError, "received non-200 response from agent-ingester",
			"status_code", resp.StatusCode,
		)

		// in the future we can pivot on StatusCode to determine if this is something that will need the
		// equivalent of reauth (e.g. a new mTLS cert)
		// reauth := resp.StatusCode == http.StatusUnauthorized
		return nil, fmt.Errorf("agent-ingester returned status %d: %s", resp.StatusCode, string(body))
	}

	return &publishLogsResponse, nil
}

func (lpc *LogPublisherClient) shouldPublishLogs() bool {
	// make sure we're fully configured to publish logs
	if lpc.knapsack.OsqueryLogPublishAPIKey() == "" || lpc.knapsack.OsqueryLogPublishURL() == "" {
		return false
	}

	dualPublicationPercentEnabled := lpc.knapsack.OsqueryLogPublishPercentEnabled()
	if dualPublicationPercentEnabled == 0 {
		return false
	}

	// generate random number between 0 and 100 to determine if this batch should be published
	// if the random number is less than the percentage enabled, publish the logs
	return rand.Intn(101) <= dualPublicationPercentEnabled
}
