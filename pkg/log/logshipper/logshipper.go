package logshipper

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/sendbuffer"
)

// TODO: consolidate this var, but where?
var observabilityIngestTokenKey = []byte("observability_ingest_auth_token")

type LogShipper struct {
	sender     *authedHttpSender
	sendBuffer *sendbuffer.SendBuffer
	logger     *logger
	knapsack   types.Knapsack
	ctx        context.Context
	cancel     context.CancelFunc
}

func New(ctx context.Context, k types.Knapsack) *LogShipper {
	token, _ := k.TokenStore().Get(observabilityIngestTokenKey)
	sender := newAuthHttpSender(logEndpoint(k), string(token))

	sendInterval := time.Minute * 1
	if k.Debug() {
		sendInterval = time.Second * 1
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))
	logger := newLogger(sendBuffer)

	thisCtx, cancel := context.WithCancel(ctx)

	return &LogShipper{
		sender:     sender,
		sendBuffer: sendBuffer,
		logger:     logger,
		knapsack:   k,
		ctx:        thisCtx,
		cancel:     cancel,
	}
}

func (ls *LogShipper) Logger() log.Logger {
	return ls.logger
}

func (ls *LogShipper) Ping() {
	token, _ := ls.knapsack.TokenStore().Get(observabilityIngestTokenKey)
	ls.sender.authtoken = string(token)
	ls.sender.endpoint = logEndpoint(ls.knapsack)
}

// StartShipping is a no-op -- the exporter is already running in the background. The TraceExporter
// otherwise only responds to control server events.
func (ls *LogShipper) StartShipping() error {
	ls.sendBuffer.Run(ls.ctx)
	return nil
}

func (ls *LogShipper) StopShipping(_ error) {
	ls.cancel()
}

func logEndpoint(k types.Knapsack) string {
	scheme := "https"
	if k.DisableObservabilityIngestTLS() {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s/log", scheme, k.ObservabilityIngestServerURL())
}
