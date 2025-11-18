package osquerypublisher

import osqlog "github.com/osquery/osquery-go/plugin/logger"

// these types are exported for re-use by agent-ingester
// in the future we will likely use a more robust schema enforcement tool but
// will utilize these types as the contract for early development
type (
	// PublishLogsRequest represents the expected JSON structure for log data. We will likely add some
	// identifying metadata here once we have a better idea of the authentication mechanism (may just be the node key
	// as it current is in our existing request structure, but there is currently no reason to send it).
	PublishLogsRequest struct {
		LogType osqlog.LogType `json:"log_type"`
		Logs    []string       `json:"logs"`
	}

	// PublishLogsResponse represents the response structure from agent-ingester for a given batch of log publications.
	// Note that this can also be used to unmarshal and errorResponse from agent-ingester, in that case
	PublishLogsResponse struct {
		Status        string `json:"status"`
		IngestedBytes int64  `json:"ingested_bytes"`
		LogCount      int    `json:"log_count"`
		Message       string `json:"message"`
	}
)
