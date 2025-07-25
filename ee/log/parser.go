package log

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const keyFormatWithPrefix = "original.%s"

// LogRawLogRecord parses the given `rawLogRecord`, which should be a JSON-encoded slog LogRecord,
// and then logs it. We use this to process logs from related subprocesses (launcher watchdog,
// launcher desktop) and log them at the correct level.
func LogRawLogRecord(ctx context.Context, rawLogRecord []byte, slogger *slog.Logger) {
	logRecord := make(map[string]any)

	if err := json.Unmarshal(rawLogRecord, &logRecord); err != nil {
		// If we can't parse the log, then log the raw string.
		slogger.Log(ctx, slog.LevelError,
			"failed to unmarshal incoming log",
			"log", string(rawLogRecord),
			"err", err,
		)
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
