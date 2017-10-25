package log

import (
	"io"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type Logger struct {
	swapLogger *log.SwapLogger
	baseLogger log.Logger
}

// TODO kill this
func (l *Logger) Log(keyvals ...interface{}) error {
	return l.swapLogger.Log(keyvals...)
}

func (l *Logger) AllowDebug() {
	l.allowedLevel(level.AllowDebug(), "debug")
}

func (l *Logger) AllowInfo() {
	l.allowedLevel(level.AllowInfo(), "info")
}

func (l *Logger) allowedLevel(lev level.Option, name string) {
	newLogger := level.NewFilter(l.baseLogger, lev)
	l.swapLogger.Swap(newLogger)
	l.Info(
		"msg", "allowed log level set",
		"allowed_level", name,
	)
}

func (l *Logger) Debug(keyvals ...interface{}) error {
	return level.Debug(l.swapLogger).Log(keyvals...)
}

func (l *Logger) Info(keyvals ...interface{}) error {
	return level.Info(l.swapLogger).Log(keyvals...)
}

func (l *Logger) Fatal(keyvals ...interface{}) error {
	level.Info(l.swapLogger).Log(keyvals...)
	os.Exit(1)
	// never hit
	return nil
}

func NewLogger(w io.Writer) *Logger {
	logger := log.NewJSONLogger(log.NewSyncWriter(w))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	// Note: the constant in log.Caller is fragile and must be set
	// appropriately based on the level of wrapping of the logger
	logger = log.With(logger, "caller", log.Caller(7))

	l := &Logger{
		swapLogger: new(log.SwapLogger),
		baseLogger: logger,
	}
	l.swapLogger.Swap(level.NewFilter(l.baseLogger, level.AllowInfo()))

	return l
}
