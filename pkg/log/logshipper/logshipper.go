package logshipper

import (
	"context"
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
	defaultSendInterval   = 1 * time.Minute
	debugSendInterval     = 1 * time.Second
)

type LogShipper struct {
	sender            *authedHttpSender
	sendBuffer        *sendbuffer.SendBuffer
	logger            log.Logger
	knapsack          types.Knapsack
	stopFunc          context.CancelFunc
	isShippingEnabled bool
}

func New(k types.Knapsack) *LogShipper {
	startEnabled := k.LogIngestServerURL() != ""

	logEndpoint, err := logEndpoint(k)
	if err != nil {
		// If we have a bad endpoint, just disable for now.
		// It will get renabled when control server sends a
		// valid endpoint.
		startEnabled = false
	}

	token, _ := k.TokenStore().Get(observabilityIngestTokenKey)
	sender := newAuthHttpSender(logEndpoint, string(token))

	sendInterval := defaultSendInterval
	if k.Debug() {
		sendInterval = debugSendInterval
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))
	logger := log.NewJSONLogger(sendBuffer)

	return &LogShipper{
		sender:            sender,
		sendBuffer:        sendBuffer,
		logger:            logger,
		knapsack:          k,
		isShippingEnabled: startEnabled,
	}
}

func (ls *LogShipper) Ping() {
	// set up new auth token
	token, _ := ls.knapsack.TokenStore().Get(observabilityIngestTokenKey)
	ls.sender.authtoken = string(token)

	shouldEnable := ls.knapsack.LogIngestServerURL() != ""
	endpoint, err := logEndpoint(ls.knapsack)
	if err != nil {
		// If we have a bad endpoint, just disable for now.
		// It will get renabled when control server sends a
		// valid endpoint.
		shouldEnable = false
	}

	ls.sender.endpoint = endpoint
	ls.isShippingEnabled = shouldEnable

	if !ls.isShippingEnabled {
		ls.sendBuffer.DeleteAllData()
	}
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
	endpoint := k.LogIngestServerURL()
	parsedUrl, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	return parsedUrl.String(), nil
}

func (ls *LogShipper) Log(keyvals ...interface{}) error {
	if !ls.isShippingEnabled {
		return nil
	}

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
