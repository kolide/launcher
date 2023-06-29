package logshipper

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/sendbuffer"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
	defaultSendInterval   = 1 * time.Minute
	debugSendInterval     = 1 * time.Second
)

type LogShipper struct {
	sender     *authedHttpSender
	sendBuffer *sendbuffer.SendBuffer
	//shippingLogger is the logs that will be shipped
	shippingLogger log.Logger
	//baseLogger is for logShipper interal logging
	baseLogger        log.Logger
	knapsack          types.Knapsack
	stopFunc          context.CancelFunc
	isShippingEnabled bool
}

func New(k types.Knapsack, baseLogger log.Logger) *LogShipper {
	sender := newAuthHttpSender()

	sendInterval := defaultSendInterval
	if k.Debug() {
		sendInterval = debugSendInterval
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))
	shippingLogger := log.With(log.NewJSONLogger(sendBuffer), "caller", log.Caller(6), "session_ulid", ulid.New())

	ls := &LogShipper{
		sender:         sender,
		sendBuffer:     sendBuffer,
		shippingLogger: shippingLogger,
		baseLogger:     baseLogger,
		knapsack:       k,
	}

	ls.Ping()
	return ls
}

// Ping gets the latest token and endpoint from knapsack and updates the sender
func (ls *LogShipper) Ping() {
	// set up new auth token
	token, _ := ls.knapsack.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	ls.sender.authtoken = string(token)

	shouldEnable := ls.knapsack.LogIngestServerURL() != ""
	parsedUrl, err := url.Parse(ls.knapsack.LogIngestServerURL())
	if err != nil || parsedUrl.String() == "" {
		// If we have a bad endpoint, just disable for now.
		// It will get renabled when control server sends a
		// valid endpoint.
		shouldEnable = false
		ls.baseLogger.Log(
			"msg", "error parsing log ingest server url, shipping disabled",
			"err", err,
			"log_ingest_url", ls.knapsack.LogIngestServerURL(),
		)
	}

	ls.sender.endpoint = parsedUrl.String()
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

func (ls *LogShipper) Log(keyvals ...interface{}) error {
	if !ls.isShippingEnabled {
		return nil
	}

	filterResults(keyvals...)
	return ls.shippingLogger.Log(keyvals...)
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
