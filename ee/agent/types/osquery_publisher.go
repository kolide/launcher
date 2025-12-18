package types

import (
	"context"

	osqlog "github.com/osquery/osquery-go/plugin/logger"
)

// OsqueryPublisher is an interface for publishing osquery logs/results.
// The concrete implementation is provided by osquerypublisher.LogPublisherClient.
type OsqueryPublisher interface {
	// Ping refreshes the token cache. also satisfies the control subscriber interface to handle token updates
	Ping()
	// PublishLogs publishes logs from the osquery process. These may be
	// status logs or result logs from scheduled queries.
	PublishLogs(ctx context.Context, logType osqlog.LogType, logs []string) (*PublishOsqueryLogsResponse, error)
}

// these types are exported for re-use by agent-ingester
// in the future we will likely use a more robust schema enforcement tool but
// will utilize these types as the contract for early development
type (
	// PublishOsqueryLogsRequest represents the expected JSON structure for log data. We will likely add some
	// identifying metadata here once we have a better idea of the authentication mechanism (may just be the node key
	// as it current is in our existing request structure, but there is currently no reason to send it).
	PublishOsqueryLogsRequest struct {
		LogType osqlog.LogType `json:"log_type"`
		Logs    []string       `json:"logs"`
	}

	// PublishOsqueryLogsResponse represents the response structure from agent-ingester for a given batch of log publications.
	// Note that this can also be used to unmarshal an errorResponse from agent-ingester
	PublishOsqueryLogsResponse struct {
		Status        string `json:"status"`
		IngestedBytes int64  `json:"ingested_bytes"`
		LogCount      int    `json:"log_count"`
		Message       string `json:"message"`
	}
)
