package osquerylogpublisher

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/kit/contexts/uuid"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/service"
	"github.com/osquery/osquery-go/plugin/logger"
)

type (
	// LogPublisherClient adheres to the Publisher interface. It handles log publication
	// to the agent-ingester microservice
	LogPublisherClient struct {
		logger   *slog.Logger
		knapsack types.Knapsack
		client   *http.Client
	}
)

func NewLogPublisherClient(logger *slog.Logger, k types.Knapsack, client *http.Client) *LogPublisherClient {
	return &LogPublisherClient{
		logger:   logger.With("component", "osquery_log_publisher"),
		knapsack: k,
		client:   client,
	}
}

func (lpc *LogPublisherClient) PublishLogs(ctx context.Context, logType logger.LogType, logs []string) (bool, error) {
	requestUUID := uuid.NewForRequest()
	ctx = uuid.NewContext(ctx, requestUUID)

	// defer func(begin time.Time) {
	// 	pubStateVals, ok := ctx.Value(service.PublicationCtxKey).(map[string]int)
	// 	if !ok {
	// 		pubStateVals = make(map[string]int)
	// 	}

	// 	lpc.knapsack.Slogger().Log(ctx, levelForError(err), message, // nolint:sloglint // it's fine to not have a constant or literal here
	// 		"method", "PublishLogs",
	// 		"uuid", requestUUID,
	// 		"logType", logType,
	// 		"log_count", len(logs),
	// 		"errcode", errcode,
	// 		"reauth", reauth,
	// 		"err", err,
	// 		"took", time.Since(begin),
	// 		"publication_state", pubStateVals,
	// 	)
	// }(time.Now())

	// use lpc.client to POST to the agent-ingester microservice /logs endpoint


	return true, nil
}
