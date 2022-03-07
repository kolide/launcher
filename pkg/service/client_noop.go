package service

import (
	"context"

	"github.com/osquery/osquery-go/plugin/distributed"
	osqlogger "github.com/osquery/osquery-go/plugin/logger"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type noopClient struct {
	logger log.Logger
}

func NewNoopClient(logger log.Logger) KolideService {
	return noopClient{
		logger: log.With(level.Info(logger), "msg", "Noop Client called"),
	}
}

func (c noopClient) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (string, bool, error) {
	c.logger.Log("method", "RequestEnrollment", "expected", "This is expected")
	return "", false, nil
}

func (c noopClient) RequestConfig(ctx context.Context, nodeKey string) (string, bool, error) {
	c.logger.Log("method", "RequestConfig")
	return "{}", false, nil
}

func (c noopClient) PublishLogs(ctx context.Context, nodeKey string, logType osqlogger.LogType, logs []string) (string, string, bool, error) {
	c.logger.Log("method", "PublishLogs")
	return "", "", false, nil
}

func (c noopClient) RequestQueries(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
	c.logger.Log("method", "RequestQueries")
	return nil, false, nil
}

func (c noopClient) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
	c.logger.Log("method", "PublishResults")
	return "", "", false, nil
}

func (c noopClient) CheckHealth(ctx context.Context) (int32, error) {
	c.logger.Log("method", "CheckHealth")
	return 0, nil
}
