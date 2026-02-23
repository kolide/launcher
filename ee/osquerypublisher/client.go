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
		authTokens  map[string]string
		hpkeKeys    map[string]*KeyData
		psks        map[string]*KeyData
		tokensMutex *sync.RWMutex // used to protect the authTokens, hpkeKeys, and psks maps
	}
)

func NewLogPublisherClient(logger *slog.Logger, k types.Knapsack, client PublisherHTTPClient) types.OsqueryPublisher {
	lpc := LogPublisherClient{
		slogger:     logger.With("component", "osquery_log_publisher"),
		knapsack:    k,
		client:      client,
		authTokens:  make(map[string]string),
		tokensMutex: &sync.RWMutex{},
		hpkeKeys:    make(map[string]*KeyData),
		psks:        make(map[string]*KeyData),
	}

	lpc.refreshTokenCache()

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
// In the future we will likely want to pass an enrollment id in here to allow for selection of
// the correct agent-ingester token to use. For now, we can use the default enrollment token.
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
// In the future we will likely want to pass an enrollment id in here to allow for selection of
// the correct agent-ingester token to use. For now, we can use the token associated with the default enrollment.
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
	var err error
	// in the future we will want to plumb an enrollment ID through here, for now just use the default
	enrollmentID := types.DefaultEnrollmentID
	authToken := lpc.getTokenForEnrollment(enrollmentID)
	if authToken == "" {
		return nil, fmt.Errorf("no auth token found for enrollment: %s", enrollmentID)
	}

	hpkeKey := lpc.getHPKEKeyForEnrollment(enrollmentID)
	if hpkeKey == nil {
		return nil, fmt.Errorf("no HPKE key available for enrollment '%s': %w", enrollmentID, err)
	}

	psk := lpc.getPSKForEnrollment(enrollmentID)
	if psk == nil {
		return nil, fmt.Errorf("no PSK available for enrollment '%s': %w", enrollmentID, err)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling unencrypted request payload: %w", err)
	}

	// TODO: (upcoming PR) data should be compressed here prior to encryption
	// encrypt the payload
	encryptedBlob, err := encryptWithHPKE(jsonData, hpkeKey, psk)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload with HPKE: %w", err)
	}

	// replace payload with encrypted blob
	jsonData, err = json.Marshal(encryptedBlob)
	if err != nil {
		return nil, fmt.Errorf("marshaling encrypted blob: %w", err)
	}

	requestUUID := uuid.NewForRequest()
	ctx = uuid.NewContext(ctx, requestUUID)
	resp := &http.Response{}
	var publicationResponse types.OsqueryPublicationResponse

	defer func(begin time.Time) {
		slogger.Log(ctx, levelForError(err), "attempted osquery publication",
			"request_uuid", requestUUID,
			"publication_type", publicationPath,
			"response", publicationResponse,
			"status_code", resp.StatusCode,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	url := fmt.Sprintf("%s/%s", lpc.knapsack.OsqueryPublisherURL(), publicationPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// set required headers and issue the request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	resp, err = lpc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("issuing publication request: %w", err)
	}
	defer resp.Body.Close()

	// read in the response and unmarshal
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading publication response: %w", err)
	}

	err = json.Unmarshal(body, &publicationResponse)
	if err != nil {
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
	lpc.refreshTokenCache()
}

// refreshTokenCache loads in the agent ingester auth token, HPKE keys, and PSKs from the TokenStore
// and stores them in our locally cached maps. for now we will only see single tokens for the default enrollments,
// in the future we will iterate the TokenStorage and grab everything with corresponding key prefixes to
// populate any configured enrollments.
func (lpc *LogPublisherClient) refreshTokenCache() {
	lpc.tokensMutex.Lock()
	defer lpc.tokensMutex.Unlock()

	newToken, err := lpc.knapsack.TokenStore().Get(storage.AgentIngesterAuthTokenKey)
	if err != nil || len(newToken) == 0 {
		lpc.slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to fetch ingester auth token from TokenStore, may not be set yet",
			"err", err,
		)
	} else {
		lpc.authTokens[types.DefaultEnrollmentID] = string(newToken)
	}

	// Load HPKE public key
	hpkeKeyData, err := lpc.knapsack.TokenStore().Get(storage.AgentIngesterHPKEPublicKey)
	// nothing we can do if we haven't set the HPKE key yet, just log a warning and continue
	if err != nil || len(hpkeKeyData) == 0 {
		lpc.slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to fetch HPKE key from TokenStore, may not be set yet",
			"err", err,
		)
	} else {
		hpkeKey, parseErr := parseKeyData(string(hpkeKeyData))
		if parseErr != nil {
			lpc.slogger.Log(context.TODO(), slog.LevelWarn,
				"failed to parse HPKE key from TokenStore",
				"err", parseErr,
			)
		} else {
			lpc.hpkeKeys[types.DefaultEnrollmentID] = hpkeKey
		}
	}

	// Load PSK
	pskData, err := lpc.knapsack.TokenStore().Get(storage.AgentIngesterHPKEPresharedKey)
	if err != nil || len(pskData) == 0 {
		lpc.slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to fetch PSK from TokenStore, may not be set yet",
			"err", err,
		)
	} else {
		psk, parseErr := parseKeyData(string(pskData))
		if parseErr != nil {
			lpc.slogger.Log(context.TODO(), slog.LevelWarn,
				"failed to parse PSK from TokenStore",
				"err", parseErr,
			)
		} else {
			lpc.psks[types.DefaultEnrollmentID] = psk
		}
	}
}

func (lpc *LogPublisherClient) getTokenForEnrollment(enrollmentID string) string {
	lpc.tokensMutex.RLock()
	defer lpc.tokensMutex.RUnlock()
	if token, ok := lpc.authTokens[enrollmentID]; ok {
		return token
	}

	return ""
}

// getHPKEKeyForEnrollment returns the HPKE key data for the given enrollment ID
func (lpc *LogPublisherClient) getHPKEKeyForEnrollment(enrollmentID string) *KeyData {
	lpc.tokensMutex.RLock()
	defer lpc.tokensMutex.RUnlock()
	if key, ok := lpc.hpkeKeys[enrollmentID]; ok {
		return key
	}

	return nil
}

// getPSKForEnrollment returns the PSK data for the given enrollment ID
func (lpc *LogPublisherClient) getPSKForEnrollment(enrollmentID string) *KeyData {
	lpc.tokensMutex.RLock()
	defer lpc.tokensMutex.RUnlock()
	if psk, ok := lpc.psks[enrollmentID]; ok {
		return psk
	}

	return nil
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
