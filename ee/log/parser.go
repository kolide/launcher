package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const keyFormatWithPrefix = "original.%s"

// https://developer.apple.com/documentation/usernotifications/unerrordomain?language=objc
var notificationErrorDomain = []byte("UNErrorDomain")

// LogRawLogRecord parses the given `rawLogRecord`, which should be a JSON-encoded slog LogRecord,
// and then logs it. We use this to process logs from related subprocesses (launcher watchdog,
// launcher desktop) and log them at the correct level.
func LogRawLogRecord(ctx context.Context, rawLogRecord []byte, slogger *slog.Logger) {
	logRecord := make(map[string]any)

	if err := json.Unmarshal(rawLogRecord, &logRecord); err != nil {
		logRawNonJsonLogRecord(ctx, rawLogRecord, slogger)
		return
	}

	// Extract the fields that we know we care about (msg, level, and time) from the
	// original log, so that we can set them correctly below. For any other fields that
	// we may have duplicate values for, prepend "original." to the key -- most notably,
	// we are handling potential duplicate `time` and `source` fields, and potentially
	// `component` fields that could conflict with attributes set on our `slogger`.
	// We do not prepend "original." to keys relating to errors or panics, to make sure they're
	// picked up correctly by our error reporting system.
	logArgs := make([]slog.Attr, len(logRecord))
	logLevel := slog.LevelInfo
	logMsg := "original log message missing"
	for k, v := range logRecord {
		switch k {
		case slog.MessageKey:
			if logMsgStr, ok := v.(string); ok {
				logMsg = logMsgStr
			}
		case slog.LevelKey:
			if logLevelString, ok := v.(string); ok {
				if err := logLevel.UnmarshalText([]byte(logLevelString)); err != nil {
					// Log that we couldn't parse the level, but proceed with the rest of parsing.
					slogger.Log(ctx, slog.LevelError,
						"could not parse incoming log with invalid level",
						"level", logLevelString,
						"err", err,
					)
				}
			}
		case "err", "stack_trace":
			logArgs = append(logArgs, slog.Any(k, v))
		default:
			logArgs = append(logArgs, slog.Any(fmt.Sprintf(keyFormatWithPrefix, k), v))
		}
	}

	// Re-issue the log using our slogger and our updated args.
	slogger.LogAttrs(ctx, logLevel, logMsg, logArgs...) // nolint:sloglint // it's fine to not have a constant or literal here
}

// logRawNonJsonLogRecord handles incoming log messages that we are unable
// to parse further. For example, sometimes we get non-JSON logs when the
// process hasn't fully set up logging yet, or when we're interacting with
// systray or another library that emits its own logs. We check for the types
// of logs that we're aware of and know are not errors to log those at a non-error
// level, and log all others at the error level.
func logRawNonJsonLogRecord(ctx context.Context, rawLogRecord []byte, slogger *slog.Logger) {
	logLevel := slog.LevelError

	// Check for macOS notification-related errors. We typically see these due to the user
	// not granting us permission to send notifications, which we can't do anything about.
	if bytes.Contains(rawLogRecord, notificationErrorDomain) {
		logLevel = slog.LevelWarn
	}

	// Log the raw log at the appropriate level
	slogger.Log(ctx, logLevel, string(rawLogRecord)) // nolint:sloglint // it's fine to not have a constant or literal here
}
