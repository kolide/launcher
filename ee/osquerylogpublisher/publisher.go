package osquerylogpublisher

import (
	"context"

	"github.com/osquery/osquery-go/plugin/logger"
)

type Publisher interface {
	// PublishLogs publishes logs from the osquery process. These may be
	// status logs or result logs from scheduled queries.
	PublishLogs(ctx context.Context, logType logger.LogType, logs []string) (success bool, err error)
	// PublishResults publishes the results of executed distributed queries.
	//PublishResults(ctx context.Context, results []distributed.Result) (bool, error)
}
