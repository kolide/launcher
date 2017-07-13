package service

import (
	"context"

	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

type KolideService interface {
	RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifer string) (string, bool, error)
	RequestConfig(ctx context.Context, nodeKey, version string) (string, bool, error)
	RequestQueries(ctx context.Context, nodeKey, version string) (*distributed.GetQueriesResult, bool, error)
	PublishLogs(ctx context.Context, nodeKey, version string, logType logger.LogType, logs []string) (string, string, bool, error)
	PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error)
}
