package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

func loggingMiddleware(logger log.Logger) func(KolideService) KolideService {
	return func(next KolideService) KolideService {
		return logmw{logger, next}
	}
}

type logmw struct {
	logger log.Logger
	next   KolideService
}

func (mw logmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		level.Debug(mw.logger).Log(
			"method", "RequestEnrollment",
			"enrollSecret", enrollSecret,
			"hostIdentifier", hostIdentifier,
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	errcode, reauth, err = mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier)
	return
}

func (mw logmw) RequestConfig(ctx context.Context, nodeKey string) (errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		level.Debug(mw.logger).Log(
			"method", "RequestConfig",
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	errcode, reauth, err = mw.next.RequestConfig(ctx, nodeKey)
	return
}

func (mw logmw) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (message, errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		level.Debug(mw.logger).Log(
			"method", "PublishLogs",
			"logType", logType,
			"log_count", len(logs),
			"message", message,
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	message, errcode, reauth, err = mw.next.PublishLogs(ctx, nodeKey, logType, logs)
	return
}

func (mw logmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	defer func(begin time.Time) {
		resJSON, _ := json.Marshal(res)
		level.Debug(mw.logger).Log(
			"method", "RequestQueries",
			"res", string(resJSON),
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	res, reauth, err = mw.next.RequestQueries(ctx, nodeKey)
	return
}

func (mw logmw) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (message, errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		resJSON, _ := json.Marshal(results)
		level.Debug(mw.logger).Log(
			"method", "PublishResults",
			"results", string(resJSON),
			"message", message,
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	message, errcode, reauth, err = mw.next.PublishResults(ctx, nodeKey, results)
	return
}
