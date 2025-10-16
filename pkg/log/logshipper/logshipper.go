package logshipper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/sendbuffer"
	slogmulti "github.com/samber/slog-multi"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
	defaultSendInterval   = 1 * time.Minute
	debugSendInterval     = 5 * time.Second
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
	isShippingStarted   bool
	slogLevel           *slog.LevelVar
	additionalSlogAttrs []slog.Attr
	startShippingChan   chan struct{}
}

func New(k types.Knapsack, baseLogger log.Logger) *LogShipper {
	sender := newAuthHttpSender()

	sendInterval := defaultSendInterval
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
		startShippingChan:   make(chan struct{}),
	}

	ls.slogLevel = new(slog.LevelVar)
	ls.slogLevel.Set(slog.LevelInfo)

	ls.knapsack.RegisterChangeObserver(ls, keys.LogShippingLevel, keys.LogIngestServerURL)

	ls.Ping()
	return ls
}

func (ls *LogShipper) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	// TODO: only make updates that are relevant to flag key changes
	// calling ping does more work than needed
	ls.Ping()
}

// Ping collects all data required to be able to start shipping logs,
// and starts the shipping process once all data has been collected.
func (ls *LogShipper) Ping() {
	ls.updateLogShippingLevel()

	if err := ls.updateSenderAuthToken(); err != nil {
		level.Debug(ls.baseLogger).Log(
			"msg", "updating auth token",
			"err", err,
		)
		return
	}

	if err := ls.updateLogIngestURL(); err != nil {
		level.Debug(ls.baseLogger).Log(
			"msg", "updating log ingest url",
			"err", err,
		)
		return
	}

	if err := ls.updateDevideIdentifyingAttributes(); err != nil {
		level.Debug(ls.baseLogger).Log(
			"msg", "updating device identifying attributes",
			"err", err,
		)
		return
	}

	if ls.isShippingStarted {
		return
	}

	ls.isShippingStarted = true
	gowrapper.Go(context.TODO(), ls.knapsack.Slogger(), func() {
		ls.startShippingChan <- struct{}{}
	})

}

func (ls *LogShipper) Run() error {
	ls.knapsack.Slogger().Log(context.Background(), slog.LevelInfo,
		"log shipper set up, waiting for required data to start shipping",
	)

	<-ls.startShippingChan

	ls.knapsack.Slogger().Log(context.Background(), slog.LevelInfo,
		"starting log shipping",
	)

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

// updateDevideIdentifyingAttributes gets device identifiers from the server-provided
// data and adds them as attributes on the logger. If shipping has not yet started and
// there are logs in the buffer, the device identifiers are added to the logs in the
// buffer.
func (ls *LogShipper) updateDevideIdentifyingAttributes() error {
	deviceInfo := make(map[string]string)

	versionInfo := version.Version()
	deviceInfo["launcher_version"] = versionInfo.Version
	deviceInfo["os"] = runtime.GOOS
	deviceInfo["osquery_version"] = ls.knapsack.CurrentRunningOsqueryVersion()

	ls.shippingLogger = log.With(ls.shippingLogger, "launcher_version", versionInfo.Version)

	for _, key := range []string{"device_id", "munemo", "organization_id"} {
		v, err := ls.knapsack.ServerProvidedDataStore().Get([]byte(key))
		if err != nil {
			return fmt.Errorf("could not get %s from server provided data: %w", key, err)
		}

		if len(v) == 0 {
			return fmt.Errorf("no value for %s in server provided data", key)
		}

		deviceInfo[key] = string(v)
	}

	// Get serial number from enrollment details
	enrollmentDetails := ls.knapsack.GetEnrollmentDetails()
	if enrollmentDetails.HardwareSerial == "" {
		return errors.New("no serial_number in enrollment details")
	}
	deviceInfo["serial_number"] = enrollmentDetails.HardwareSerial

	var deviceInfoKvps []any
	for k, v := range deviceInfo {
		deviceInfoKvps = append(deviceInfoKvps, k, v)
	}

	ls.shippingLogger = log.With(ls.shippingLogger, deviceInfoKvps...)

	var additionalSlogAttrs []slog.Attr
	for k, v := range deviceInfo {
		additionalSlogAttrs = append(additionalSlogAttrs, slog.String(k, v))
	}

	ls.additionalSlogAttrs = additionalSlogAttrs

	// if we are already shipping, dont update send buffer data
	if ls.isShippingStarted {
		return nil
	}

	// if this is the first time we've gotten device data, update the send buffer
	// logs to include it
	ls.sendBuffer.UpdateData(func(in io.Reader, out io.Writer) error {
		var logMap map[string]interface{}

		if err := json.NewDecoder(in).Decode(&logMap); err != nil {
			ls.shippingLogger.Log(
				"msg", "failed to decode log data",
				"err", err,
			)

			return err
		}

		for k, v := range deviceInfo {
			logMap[k] = v
		}

		if err := json.NewEncoder(out).Encode(logMap); err != nil {
			ls.shippingLogger.Log(
				"msg", "failed to encode log data",
				"err", err,
			)

			return err
		}

		return nil
	})

	return nil
}

func (ls *LogShipper) updateSenderAuthToken() error {
	token, err := ls.knapsack.TokenStore().Get(storage.ObservabilityIngestAuthTokenKey)
	if err != nil {
		return err
	}

	if len(token) == 0 {
		return errors.New("no token found")
	}

	ls.sender.authtoken = string(token)
	return nil
}

func (ls *LogShipper) updateLogIngestURL() error {
	parsedUrl, err := url.Parse(ls.knapsack.LogIngestServerURL())

	if err != nil {
		return err
	}

	if parsedUrl == nil || parsedUrl.String() == "" {
		ls.sender.endpoint = ""
		return errors.New("log ingest url is empty")
	}

	ls.sender.endpoint = parsedUrl.String()
	return nil
}

func (ls *LogShipper) updateLogShippingLevel() {
	startingLevel := ls.slogLevel.Level()
	sendInterval := defaultSendInterval

	switch ls.knapsack.LogShippingLevel() {
	case "debug":
		ls.slogLevel.Set(slog.LevelDebug)
		// if we using debug level logging, send logs more frequently
		sendInterval = debugSendInterval
	case "info":
		ls.slogLevel.Set(slog.LevelInfo)
	case "warn":
		ls.slogLevel.Set(slog.LevelWarn)
	case "error":
		ls.slogLevel.Set(slog.LevelError)
	default:
		ls.knapsack.Slogger().Log(context.TODO(), slog.LevelError,
			"unrecognized flag value for log shipping level",
			"flag_value", ls.knapsack.LogShippingLevel(),
			"current_log_level", ls.slogLevel.String(),
		)
	}

	if startingLevel != ls.slogLevel.Level() {
		ls.knapsack.Slogger().Log(context.TODO(), slog.LevelInfo,
			"log shipping level changed",
			"old_log_level", startingLevel.String(),
			"new_log_level", ls.slogLevel.Level().String(),
			"send_interval", sendInterval.String(),
		)
	}

	ls.sendBuffer.SetSendInterval(sendInterval)
}
