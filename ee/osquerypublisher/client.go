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
	"sync"
	"time"

	"github.com/kolide/kit/contexts/uuid"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/osquery/osquery-go/plugin/distributed"
	osqlog "github.com/osquery/osquery-go/plugin/logger"
)

type (
	// LogPublisherClient adheres to the Publisher interface. It handles log publication
	// to the agent-ingester microservice
	LogPublisherClient struct {
		slogger     *slog.Logger
		knapsack    types.Knapsack
		client      PublisherHTTPClient
		tokens      map[string]string
		tokensMutex *sync.RWMutex
	}
)

func NewLogPublisherClient(logger *slog.Logger, k types.Knapsack, client PublisherHTTPClient) types.OsqueryPublisher {
	lpc := LogPublisherClient{
		slogger:     logger.With("component", "osquery_log_publisher"),
		knapsack:    k,
		client:      client,
		tokens:      make(map[string]string),
		tokensMutex: &sync.RWMutex{},
	}

	if err := lpc.refreshTokenCache(); err != nil {
		lpc.slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to refresh token cache on log publisher client initialization, may not be set yet",
			"err", err,
		)
	}

	return &lpc
}

// NewPublisherHTTPClient is a helper method to allow us to make any http client tweaks as we learn realistic
// parameters for interacting with the agent-ingester service
func NewPublisherHTTPClient() PublisherHTTPClient {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// PublishLogs publishes logs to the agent-ingester service.
// It returns the response from the agent-ingester service and any error that occurred.
// In the future we will likely want to pass a registration id in here to allow for selection of
// the correct agent-ingester token to use. For now, we can use the default registration token.
func (lpc *LogPublisherClient) PublishLogs(ctx context.Context, logType osqlog.LogType, logs []string) (*types.OsqueryPublicationResponse, error) {
	if !lpc.shouldPublishLogs() {
		return nil, nil
	}

	logger := lpc.slogger.With(
		"log_type", logType.String(),
		"log_count", len(logs),
	)

	// TODO batching logic here
	payload := types.PublishOsqueryLogsRequest{
		LogType: logType,
		Logs:    logs,
	}

	return lpc.publish(ctx, logger, payload, "logs")
}

// PublishResults publishes results to the agent-ingester service.
// It returns the response from the agent-ingester service and any error that occurred.
// In the future we will likely want to pass a registration id in here to allow for selection of
// the correct agent-ingester token to use. For now, we can use the default registration token.
func (lpc *LogPublisherClient) PublishResults(ctx context.Context, results []distributed.Result) (*types.OsqueryPublicationResponse, error) {
	if !lpc.shouldPublishLogs() {
		return nil, nil
	}

	logger := lpc.slogger.With(
		"result_count", len(results),
	)

	// TODO batching logic here
	payload := types.PublishOsqueryResultsRequest{
		Results: results,
	}

	return lpc.publish(ctx, logger, payload, "results")
}

// TODO monday finish splitting this out and refactor the PublishLogs function to use this, then add batching logic
func (lpc *LogPublisherClient) publish(ctx context.Context, slogger *slog.Logger, payload any, publicationPath string) (*types.OsqueryPublicationResponse, error) {
	// in the future we will want to plumb a registration ID through here, for now just use the default
	registrationID := types.DefaultRegistrationID
	authToken := lpc.getTokenForRegistration(registrationID)
	if authToken == "" {
		return nil, fmt.Errorf("no auth token found for registration: %s", registrationID)
	}

	requestUUID := uuid.NewForRequest()
	ctx = uuid.NewContext(ctx, requestUUID)
	logger := lpc.slogger.With(
		"request_uuid", requestUUID,
	)
	var resp *http.Response
	var publicationResponse types.OsqueryPublicationResponse
	var err error

	defer func(begin time.Time) {
		logger.Log(ctx, levelForError(err), "attempted log publication",
			"publication_type", publicationPath,
			"response", publicationResponse,
			"status_code", resp.StatusCode,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to marshal log publish request",
			"err", err,
		)
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", lpc.knapsack.OsqueryPublisherURL(), publicationPath)
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
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

	err = json.Unmarshal(body, &publicationResponse)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to unmarshal response body",
			"status_code", resp.StatusCode,
			"err", err,
		)
		return nil, fmt.Errorf("unable to unmarshal agent-ingester response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// in the future we can pivot on StatusCode to determine if this is something that will need the
		// equivalent of reauth (e.g. a new mTLS cert)
		// reauth := resp.StatusCode == http.StatusUnauthorized
		return nil, fmt.Errorf("agent-ingester returned status %d: %s", resp.StatusCode, string(body))
	}

	return &publicationResponse, nil
}

func (lpc *LogPublisherClient) Ping() {
	if err := lpc.refreshTokenCache(); err != nil {
		lpc.slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to refresh token cache after ping",
			"err", err,
		)
	}
}

// refreshTokenCache loads in the agent ingester auth token from the TokenStore and stores it in
// our locally cached map
func (lpc *LogPublisherClient) refreshTokenCache() error {
	// for now we will only see a single token for the default registration, in the future we
	// will iterate the TokenStorage and grab everything with a key prefix of storage.AgentIngesterAuthTokenKey
	newToken, err := lpc.knapsack.TokenStore().Get(storage.AgentIngesterAuthTokenKey)
	if err != nil || len(newToken) == 0 {
		return fmt.Errorf("error loading token from TokenStore: %w", err)
	}

	lpc.tokensMutex.Lock()
	defer lpc.tokensMutex.Unlock()

	lpc.tokens[types.DefaultRegistrationID] = string(newToken)
	return nil
}

func (lpc *LogPublisherClient) getTokenForRegistration(registrationID string) string {
	lpc.tokensMutex.RLock()
	defer lpc.tokensMutex.RUnlock()
	if token, ok := lpc.tokens[registrationID]; ok {
		return token
	}

	return ""
}

func (lpc *LogPublisherClient) shouldPublishLogs() bool {
	// make sure we're fully configured to publish logs
	if lpc.knapsack.OsqueryPublisherURL() == "" {
		return false
	}

	dualPublicationPercentEnabled := lpc.knapsack.OsqueryPublisherPercentEnabled()
	if dualPublicationPercentEnabled == 0 {
		return false
	}

	// generate random number between 0 and 100 to determine if this batch should be published
	// if the random number is less than the percentage enabled, publish the logs
	return rand.Intn(101) <= dualPublicationPercentEnabled
}
