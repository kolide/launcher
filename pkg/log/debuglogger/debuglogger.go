package debuglogger

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type debugLogger struct {
	logger log.Logger
}

func NewKitLogger(logFilePath string) log.Logger {
	// This is meant as an always available debug tool. Thus we hardcode these options
	lj := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    3, // megabytes
		Compress:   true,
		MaxBackups: 5,
	}

	dl := debugLogger{
		logger: log.With(
			log.NewJSONLogger(log.NewSyncWriter(lj)),
			"ts", log.DefaultTimestampUTC,
			"caller", log.DefaultCaller, ///log.Caller(6),
		),
	}

	return dl
}

func (dl debugLogger) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return dl.logger.Log(keyvals...)
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
