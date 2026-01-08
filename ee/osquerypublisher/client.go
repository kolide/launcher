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

const (
	// maxRequestSizeBytes is the maximum size in bytes for a single PublishOsqueryLogsRequest.
	// Requests exceeding this size will be split into multiple smaller batches, to keep the requests
	// performant for transfer via kafka later
	maxRequestSizeBytes = 1024 * 1024 // 1MB

	// publicationPathLogs is the path for publishing logs to the agent-ingester service
	publicationPathLogs = "logs"
	// publicationPathResults is the path for publishing results to the agent-ingester service
	publicationPathResults = "results"
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

	batches := batchRequest(logs, lpc.slogger)
	logger := lpc.slogger.With(
		"log_type", logType.String(),
		"log_count", len(logs),
		"batch_count", len(batches),
	)

	pubResponse := types.OsqueryPublicationResponse{}

	for idx, logBatch := range batches {
		payload := types.PublishOsqueryLogsRequest{
			LogType: logType,
			Logs:    logBatch,
		}

		resp, err := lpc.publish(ctx, logger, payload, publicationPathLogs)
		if err != nil {
			logger.Log(ctx, slog.LevelError, "encountered error publishing log batch",
				"err", err,
				"batch_index", idx,
			)

			return nil, err
		}

		pubResponse.IngestedBytes += resp.IngestedBytes
		pubResponse.LogCount += resp.LogCount
		pubResponse.Status = resp.Status
	}

	return &pubResponse, nil
}

// PublishResults publishes results to the agent-ingester service.
// It returns the response from the agent-ingester service and any error that occurred.
// In the future we will likely want to pass a registration id in here to allow for selection of
// the correct agent-ingester token to use. For now, we can use the default registration token.
func (lpc *LogPublisherClient) PublishResults(ctx context.Context, results []distributed.Result) (*types.OsqueryPublicationResponse, error) {
	if !lpc.shouldPublishLogs() {
		return nil, nil
	}

	batches := batchRequest(results, lpc.slogger)
	logger := lpc.slogger.With(
		"result_count", len(results),
		"batch_count", len(batches),
	)

	pubResponse := types.OsqueryPublicationResponse{}

	for idx, resultBatch := range batches {
		payload := types.PublishOsqueryResultsRequest{
			Results: resultBatch,
		}

		resp, err := lpc.publish(ctx, logger, payload, publicationPathResults)
		if err != nil {
			logger.Log(ctx, slog.LevelError, "encountered error publishing results batch",
				"err", err,
				"batch_index", idx,
			)

			return nil, err
		}

		pubResponse.IngestedBytes += resp.IngestedBytes
		pubResponse.LogCount += resp.LogCount
		pubResponse.Status = resp.Status
	}

	return &pubResponse, nil
}

// publish handles the common logic for publishing logs and results to the agent-ingester service. This
// includes marshalling the payload, fetching the auth token, issuing the request, and handling the response/logging.
func (lpc *LogPublisherClient) publish(ctx context.Context, slogger *slog.Logger, payload any, publicationPath string) (*types.OsqueryPublicationResponse, error) {
	// in the future we will want to plumb a registration ID through here, for now just use the default
	registrationID := types.DefaultRegistrationID
	authToken := lpc.getTokenForRegistration(registrationID)
	if authToken == "" {
		return nil, fmt.Errorf("no auth token found for registration: %s", registrationID)
	}

	requestUUID := uuid.NewForRequest()
	ctx = uuid.NewContext(ctx, requestUUID)
	logger := slogger.With(
		"request_uuid", requestUUID,
		"publication_type", publicationPath,
	)
	var resp *http.Response
	var publicationResponse types.OsqueryPublicationResponse
	var err error

	defer func(begin time.Time) {
		logger.Log(ctx, levelForError(err), "attempted osquery publication",
			"response", publicationResponse,
			"status_code", resp.StatusCode,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Log(ctx, slog.LevelError,
			"failed to marshal publication request",
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

// batchRequest takes in a slice of logs or distributed results and returns a slice of slices of either
// that will fit within maxRequestSizeBytes (set for kafka performance). If a single log/result exceeds the max request size,
// it is added as its own batch.
func batchRequest[Measureable string | distributed.Result](logs []Measureable, logger *slog.Logger) [][]Measureable {
	logger = logger.With("batch_type", fmt.Sprintf("%T", logs))
	batches := make([][]Measureable, 0)
	currentLogBatchSize := 0
	currentBatch := make([]Measureable, 0)
	logLength := 0
	for _, log := range logs {
		// marshal the result to get the length of the raw bytes
		rawLog, err := json.Marshal(log)
		if err != nil { // this should never happen, just log and continue if so
			logger.Log(context.TODO(), slog.LevelError,
				"failed to marshal osquery result",
				"err", err,
			)
			continue
		}
		logLength = len(rawLog)
		// if a single log/result ever exceeds the max request size, add as its own batch and log
		// this loudly, this is not expected and may cause issues downstream
		if logLength > maxRequestSizeBytes {
			logger.Log(context.TODO(), slog.LevelWarn,
				"single osquery log or result exceeds max request size",
				"log_length", logLength,
				"max_request_size", maxRequestSizeBytes,
			)
			// add the log as its own batch but don't alter any of the current batch size state, that can continue
			// in case there are other smaller logs that can still fit in the current batch
			batches = append(batches, []Measureable{log})
			continue
		}
		// if the size of the next log would exceed the max request size, finalize the existing and start a new batch
		if currentLogBatchSize+logLength > maxRequestSizeBytes {
			batches = append(batches, currentBatch)
			currentBatch = make([]Measureable, 0)
			currentLogBatchSize = 0
		}

		currentBatch = append(currentBatch, log)
		currentLogBatchSize += logLength
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}
