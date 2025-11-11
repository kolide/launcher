package osquerylogpublisher

import (
	"context"
	"log/slog"
	"net/http"

	osqlog "github.com/osquery/osquery-go/plugin/logger"
)

type (
	Publisher interface {
		// PublishLogs publishes logs from the osquery process. These may be
		// status logs or result logs from scheduled queries.
		PublishLogs(ctx context.Context, logType osqlog.LogType, logs []string) (*PublishLogsResponse, error)
		// PublishResults publishes the results of executed distributed queries.
		//PublishResults(ctx context.Context, results []distributed.Result) (*PublishResultsResponse, error)
	}

	PublisherHTTPClient interface {
		Do(req *http.Request) (*http.Response, error)
	}

	// PublishLogsRequest represents the expected JSON structure for log data. We will likely add some
	// identifying metadata here once we have a better idea of the authentication mechanism (may just be the node key
	// as it current is in our existing request structure, but there is currently no reason to send it).
	// This is exported for use by agent-ingester
	PublishLogsRequest struct {
		LogType osqlog.LogType `json:"log_type"`
		Logs    []string       `json:"logs"`
	}

	// PublishLogsResponse represents the response structure from agent-ingester for a given batch of log publications.
	// Note that this can also be used to unmarshal and errorResponse from agent-ingester, in that case
	// only the status and message fields will be populated. This is exported here for use by agent-ingester.
	PublishLogsResponse struct {
		Status        string `json:"status"`
		IngestedBytes int64  `json:"ingested_bytes"`
		LogCount      int    `json:"log_count"`
		Message       string `json:"message"`
	}
)

// levelForError returns slog.LevelWarn if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelWarn
	}
	return slog.LevelDebug
}
