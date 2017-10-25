package log

import (
	"io"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// Logger is the interface exposed to users of this launcher logger package.
type Logger interface {
	// Debug logs keyvals at the "debug" level
	Debug(keyvals ...interface{}) error
	// Info logs keyvals at the "info" level
	Info(keyvals ...interface{}) error
	// Fatal logs keyvals at the "info" level, and then exits with error code 1.
	Fatal(keyvals ...interface{}) error

	// AllowDebug sets the allowed log level to include debug logs and above (info).
	AllowDebug()
	// AllowInfo sets the allowed log level to include info logs.
	AllowInfo()
}

type logger struct {
	// swapLogger stores the leveled logger, using go-kit's SwapLogger to
	// ensure that updates to the logger are atomic and avoid race
	// conditions. All logging should be done through swapLogger.
	swapLogger *log.SwapLogger
	// baseLogger stores the un-leveled logger. Writes should not be done
	// directly to this logger.
	baseLogger log.Logger
}

func (l *logger) AllowDebug() {
	l.allowedLevel(level.AllowDebug(), "debug")
}

func (l *logger) AllowInfo() {
	l.allowedLevel(level.AllowInfo(), "info")
}

func (l *logger) allowedLevel(lev level.Option, name string) {
	newLogger := level.NewFilter(l.baseLogger, lev)
	l.swapLogger.Swap(newLogger)
	l.Info(
		"msg", "allowed log level set",
		"allowed_level", name,
	)
}

func (l *logger) Debug(keyvals ...interface{}) error {
	return level.Debug(l.swapLogger).Log(keyvals...)
}

func (l *logger) Info(keyvals ...interface{}) error {
	return level.Info(l.swapLogger).Log(keyvals...)
}

func (l *logger) Fatal(keyvals ...interface{}) error {
	// Call this directly instead of using l.Info so that we get the
	// correct caller.
	level.Info(l.swapLogger).Log(keyvals...)
	os.Exit(1)
	// never hit
	return nil
}

func NewLogger(w io.Writer) Logger {
	base := log.NewJSONLogger(log.NewSyncWriter(w))
	base = log.With(base, "ts", log.DefaultTimestampUTC)

	// The constant in log.Caller is fragile and must be set
	// appropriately based on the level of wrapping of the logger. If the
	// wrapping changes and this value becomes set incorrectly, TestCaller
	// should fail.
	base = log.With(base, "caller", log.Caller(7))

	l := &logger{
		swapLogger: new(log.SwapLogger),
		baseLogger: base,
	}
	l.swapLogger.Swap(level.NewFilter(l.baseLogger, level.AllowInfo()))

	return l
}
