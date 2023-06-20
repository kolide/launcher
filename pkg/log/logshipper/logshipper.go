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
}

func New(k types.Knapsack) *LogShipper {
	token, _ := k.TokenStore().Get(observabilityIngestTokenKey)
	sender := newAuthHttpSender(logEndpoint(k.ObservabilityIngestServerURL()), string(token))

	sendInterval := time.Minute * 5
	if k.Debug() {
		// get a fast interval if we're debugging so we
		sendInterval = time.Second * 1
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))
	logger := newLogger(sendBuffer)

	return &LogShipper{
		sender:     sender,
		sendBuffer: sendBuffer,
		logger:     logger,
		knapsack:   k,
	}
}

func (ls *LogShipper) Logger() log.Logger {
	return ls.logger
}

func (ls *LogShipper) Ping() {
	token, _ := ls.knapsack.TokenStore().Get(observabilityIngestTokenKey)
	ls.sender.authtoken = string(token)
	ls.sender.endpoint = logEndpoint(ls.knapsack.ObservabilityIngestServerURL())
}

// StartShipping is a no-op -- the exporter is already running in the background. The TraceExporter
// otherwise only responds to control server events.
func (ls *LogShipper) StartShipping() error {
	ls.sendBuffer.StartSending()

	// nothing else to do, wait for launcher to exit
	<-context.Background().Done()
	return nil
}

func (t *LogShipper) StopShipping(_ error) {
	t.sendBuffer.StopSending()
}

func logEndpoint(url string) string {
	// TODO: add new value for logging endpoint
	return fmt.Sprintf("http://localhost:8080/log")
}
