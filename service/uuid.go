package service

import (
	"context"

	goog_uuid "github.com/google/uuid"
	"github.com/kolide/launcher/service/uuid"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

func uuidMiddleware(next KolideService) KolideService {
	return uuidmw{next}
}

type uuidmw struct {
	next KolideService
}

func makeUUID() string {
	uuid, err := goog_uuid.NewRandom()
	if err != nil {
		return ""
	}

	return uuid.String()
}

func (mw uuidmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier)
}

func (mw uuidmw) RequestConfig(ctx context.Context, nodeKey string) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.RequestConfig(ctx, nodeKey)
}

func (mw uuidmw) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (message, errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.PublishLogs(ctx, nodeKey, logType, logs)
}

func (mw uuidmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.RequestQueries(ctx, nodeKey)
}

func (mw uuidmw) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (message, errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.PublishResults(ctx, nodeKey, results)
}

func (mw uuidmw) CheckHealth(ctx context.Context) (status int32, err error) {
	ctx = uuid.NewContext(ctx, makeUUID())
	return mw.next.CheckHealth(ctx)
}
