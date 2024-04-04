package locallogger

import (
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type localLogger struct {
	logger log.Logger
	writer io.Writer
}

func NewKitLogger(logFilePath string) localLogger {
	// This is meant as an always available debug tool. Thus we hardcode these options
	lj := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    3, // megabytes
		Compress:   true,
		MaxBackups: 5,
	}

	writer := log.NewSyncWriter(lj)

	ll := localLogger{
		logger: log.With(
			log.NewJSONLogger(writer),
			"ts", log.DefaultTimestampUTC,
			"caller", log.DefaultCaller, ///log.Caller(6),
		),
		writer: writer,
	}

	return ll
}

func (ll localLogger) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return ll.logger.Log(keyvals...)
}

func (ll localLogger) Writer() io.Writer {
	return ll.writer
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
