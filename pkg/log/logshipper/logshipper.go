package logshipper

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/sendbuffer"
	slogmulti "github.com/samber/slog-multi"
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
	//baseLogger is for logShipper internal logging
	baseLogger          log.Logger
	knapsack            types.Knapsack
	stopFunc            context.CancelFunc
	stopFuncMutex       sync.Mutex
	isShippingEnabled   bool
	slogLevel           *slog.LevelVar
	additionalSlogAttrs []slog.Attr
}

func New(k types.Knapsack, baseLogger log.Logger) *LogShipper {
	sender := newAuthHttpSender()

	sendInterval := defaultSendInterval
	if k.Debug() {
		sendInterval = debugSendInterval
	}

	sendBuffer := sendbuffer.New(sender, sendbuffer.WithSendInterval(sendInterval))

	// setting a ulid as session_ulid allows us to follow a single run of launcher
	shippingLogger := log.With(log.NewJSONLogger(sendBuffer), "caller", log.Caller(6), "session_ulid", ulid.New())

	ls := &LogShipper{
		sender:              sender,
		sendBuffer:          sendBuffer,
		shippingLogger:      shippingLogger,
		baseLogger:          log.With(baseLogger, "component", "logshipper"),
		knapsack:            k,
		stopFuncMutex:       sync.Mutex{},
		additionalSlogAttrs: make([]slog.Attr, 0),
	}

	ls.slogLevel = new(slog.LevelVar)
	ls.slogLevel.Set(slog.LevelError)

	ls.Ping()
	return ls
}

// Ping gets the latest token and endpoint from knapsack and updates the sender
func (ls *LogShipper) Ping() {
	// set up new auth token
	token, _ := ls.knapsack.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	ls.sender.authtoken = string(token)

	parsedUrl, err := url.Parse(ls.knapsack.LogIngestServerURL())
	if err != nil {
		// If we have a bad endpoint, just disable for now.
		// It will get renabled when control server sends a
		// valid endpoint.
		ls.sender.endpoint = ""
		level.Debug(ls.baseLogger).Log(
			"msg", "error parsing log ingest server url, shipping disabled",
			"err", err,
			"log_ingest_url", ls.knapsack.LogIngestServerURL(),
		)
	} else if parsedUrl != nil {
		ls.sender.endpoint = parsedUrl.String()
	}

	startingLevel := ls.slogLevel.Level()
	switch ls.knapsack.LogShippingLevel() {
	case "debug":
		ls.slogLevel.Set(slog.LevelDebug)
	case "info":
		ls.slogLevel.Set(slog.LevelInfo)
	case "warn":
		ls.slogLevel.Set(slog.LevelWarn)
	case "error":
		ls.slogLevel.Set(slog.LevelError)
	case "default":
		ls.knapsack.Slogger().Error("unrecognized flag value for log shipping level",
			"flag_value", ls.knapsack.LogShippingLevel(),
			"current_log_level", ls.slogLevel.String(),
		)
	}

	if startingLevel != ls.slogLevel.Level() {
		ls.knapsack.Slogger().Info("log shipping level changed",
			"old_log_level", startingLevel.String(),
			"new_log_level", ls.slogLevel.Level().String(),
		)
	}

	ls.isShippingEnabled = ls.sender.endpoint != ""
	ls.addDeviceIdentifyingAttributesToLogger()

	if !ls.isShippingEnabled {
		ls.sendBuffer.DeleteAllData()
	}
}

func (ls *LogShipper) Run() error {
	ctx, cancel := context.WithCancel(context.Background())

	ls.stopFuncMutex.Lock()
	ls.stopFunc = cancel
	ls.stopFuncMutex.Unlock()

	return ls.sendBuffer.Run(ctx)
}

func (ls *LogShipper) Stop(_ error) {
	ls.stopFuncMutex.Lock()
	defer ls.stopFuncMutex.Unlock()

	if ls.stopFunc != nil {
		ls.stopFunc()
	}
}

func (ls *LogShipper) Log(keyvals ...interface{}) error {
	if !ls.isShippingEnabled {
		return nil
	}

	filterResults(keyvals...)
	return ls.shippingLogger.Log(keyvals...)
}

func (ls *LogShipper) SlogHandler() slog.Handler {
	middleware := slogmulti.NewHandleInlineMiddleware(func(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
		record.AddAttrs(ls.additionalSlogAttrs...)
		return next(ctx, record)
	})

	jsonHandler := slog.NewJSONHandler(ls.sendBuffer, &slog.HandlerOptions{
		Level:     ls.slogLevel,
		AddSource: true,
	})

	return slogmulti.Pipe(middleware).Handler(jsonHandler)
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

// addDeviceIdentifyingAttributesToLogger gets device identifiers from the server-provided
// data and adds them as attributes on the logger.
func (ls *LogShipper) addDeviceIdentifyingAttributesToLogger() {
	additionalSlogAttrs := make([]slog.Attr, 0)

	versionInfo := version.Version()
	ls.shippingLogger = log.With(ls.shippingLogger, "version", versionInfo.Version)
	additionalSlogAttrs = append(additionalSlogAttrs, slog.Attr{Key: "version", Value: slog.StringValue(versionInfo.Version)})

	if deviceId, err := ls.knapsack.ServerProvidedDataStore().Get([]byte("device_id")); err != nil {
		level.Debug(ls.baseLogger).Log("msg", "could not get device id", "err", err)
	} else {
		ls.shippingLogger = log.With(ls.shippingLogger, "k2_device_id", string(deviceId))
		additionalSlogAttrs = append(additionalSlogAttrs, slog.Attr{Key: "k2_device_id", Value: slog.StringValue(string(deviceId))})
	}

	if munemo, err := ls.knapsack.ServerProvidedDataStore().Get([]byte("munemo")); err != nil {
		level.Debug(ls.baseLogger).Log("msg", "could not get munemo", "err", err)
	} else {
		ls.shippingLogger = log.With(ls.shippingLogger, "k2_munemo", string(munemo))
		additionalSlogAttrs = append(additionalSlogAttrs, slog.Attr{Key: "k2_munemo", Value: slog.StringValue(string(munemo))})
	}

	if orgId, err := ls.knapsack.ServerProvidedDataStore().Get([]byte("organization_id")); err != nil {
		level.Debug(ls.baseLogger).Log("msg", "could not get organization id", "err", err)
	} else {
		ls.shippingLogger = log.With(ls.shippingLogger, "k2_organization_id", string(orgId))
		additionalSlogAttrs = append(additionalSlogAttrs, slog.Attr{Key: "k2_organization_id", Value: slog.StringValue(string(orgId))})
	}

	if serialNumber, err := ls.knapsack.ServerProvidedDataStore().Get([]byte("serial_number")); err != nil {
		level.Debug(ls.baseLogger).Log("msg", "could not get serial number", "err", err)
	} else {
		ls.shippingLogger = log.With(ls.shippingLogger, "serial_number", string(serialNumber))
		additionalSlogAttrs = append(additionalSlogAttrs, slog.Attr{Key: "serial_number", Value: slog.StringValue(string(serialNumber))})
	}

	ls.additionalSlogAttrs = additionalSlogAttrs
}
