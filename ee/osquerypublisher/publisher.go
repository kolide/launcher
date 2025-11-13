package osquerypublisher

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
)

// levelForError returns slog.LevelWarn if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelWarn
	}
	return slog.LevelDebug
}
