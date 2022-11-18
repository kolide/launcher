package locallogger

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type localLogger struct {
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

	ll := localLogger{
		logger: log.With(
			log.NewJSONLogger(log.NewSyncWriter(lj)),
			"ts", log.DefaultTimestampUTC,
			"caller", log.DefaultCaller, ///log.Caller(6),
		),
	}

	return ll
}

func (ll localLogger) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return ll.logger.Log(keyvals...)
}

func CleanUpRenamedDebugLogs(cleanupPath string, logger log.Logger) {
	// We renamed the debug log file from debug.log to debug.json for compatibility with support tools.
	// Check to see if we have any of the old debug.log files still hanging around, and clean them up
	// if so. The current one is always named debug.log, and rotated files are in the format
	// `debug-<date>.log.gz`. We do not return an error if we can't clean up these files -- it's not
	// a big deal.
	legacyDebugLogPattern := filepath.Join(cleanupPath, "debug*.log*")
	filesToCleanUp, err := filepath.Glob(legacyDebugLogPattern)
	if err != nil {
		level.Error(logger).Log("msg", "could not glob for legacy debug log files to clean up", "pattern", legacyDebugLogPattern, "err", err)
	} else {
		for _, fileToCleanUp := range filesToCleanUp {
			if err := os.Remove(fileToCleanUp); err != nil {
				level.Error(logger).Log("msg", "could not clean up legacy debug log file", "file", fileToCleanUp, "err", err)
			}
		}
	}
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
