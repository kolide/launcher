package logshipper

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/sendbuffer"
)

// TODO: consolidate this var, but where?
var observabilityIngestTokenKey = []byte("observability_ingest_auth_token")

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type LogShipper struct {
	sender     *authedHttpSender
	sendBuffer *sendbuffer.SendBuffer
	logger     log.Logger
	knapsack   types.Knapsack
	stopFunc   context.CancelFunc
}

func New(k types.Knapsack) (*LogShipper, error) {
	token, _ := k.TokenStore().Get(observabilityIngestTokenKey)

	logEndpoint, err := logEndpoint(k)
	if err != nil {
		return nil, err
	}

	sender := newAuthHttpSender(logEndpoint, string(token))

	sendInterval := time.Minute * 1
	if k.Debug() {
		sendInterval = time.Second * 1
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))
	logger := log.NewJSONLogger(sendBuffer)

	return &LogShipper{
		sender:     sender,
		sendBuffer: sendBuffer,
		logger:     logger,
		knapsack:   k,
	}, nil
}

func (ls *LogShipper) Ping() {
	token, _ := ls.knapsack.TokenStore().Get(observabilityIngestTokenKey)
	ls.sender.authtoken = string(token)

	endpoint, err := logEndpoint(ls.knapsack)
	if err != nil {
		ls.logger.Log("msg", "failed to get endpoint", "err", err)
		return
	}
	ls.sender.endpoint = endpoint
}

func (ls *LogShipper) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	ls.stopFunc = cancel
	return ls.sendBuffer.Run(ctx)
}

func (ls *LogShipper) Stop(_ error) {
	ls.stopFunc()
}

func logEndpoint(k types.Knapsack) (string, error) {
	u, err := url.Parse(k.ObservabilityIngestServerURL())
	if err != nil {
		return "", err
	}

	if u.Scheme == "" {
		u.Scheme = "https"

		if k.DisableObservabilityIngestTLS() {
			u.Scheme = "http"
		}
	}

	if u.Scheme != "https" && !k.DisableObservabilityIngestTLS() {
		return "", errors.New("logshipper requires HTTPS")
	}

	return u.String(), nil
}

func (ls *LogShipper) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return ls.logger.Log(keyvals...)
}

// filterResults filteres out the osquery results,
// which just make a lot of noise in our debug logs.
// It's a bit fragile, since it parses keyvals, but
// hopefully that's good enough
func filterResults(keyvals ...interface{}) {
	// Consider switching on `method` as well?
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "results" && len(keyvals) > i+1 {
			str, ok := keyvals[i+1].(string)
			if ok && len(str) > 100 {
				keyvals[i+1] = fmt.Sprintf(truncatedFormatString, str[0:99])
			}
		}
	}
}
