package log

import (
	"io"
	"os"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// Logger is the interface exposed to users of this launcher logger package.
type Logger interface {
	kitlog.Logger
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
	// baseLogger stores the un-leveled logger. Writes should not be done
	// directly to this logger.
	baseLogger kitlog.Logger
	// swapLogger stores the leveled logger, using go-kit's SwapLogger to
	// ensure that updates to the logger are atomic and avoid race
	// conditions. All logging should be done through swapLogger.
	swapLogger kitlog.SwapLogger
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

// Log will log with level: unknown (filtered as if it were info). Prefer using
// an explicitly leveled log.Debug or log.Info.
func (l *logger) Log(keyvals ...interface{}) error {
	return level.Info(&l.swapLogger).Log(append(keyvals, "level", "unknown")...)
}

// Debug will log with level: debug
func (l *logger) Debug(keyvals ...interface{}) error {
	return level.Debug(&l.swapLogger).Log(keyvals...)
}

// Info will log with level: info
func (l *logger) Info(keyvals ...interface{}) error {
	return level.Info(&l.swapLogger).Log(keyvals...)
}

// Fatal will log with level: fatal and exit with a status 1
func (l *logger) Fatal(keyvals ...interface{}) error {
	// Call this directly instead of using l.Info so that we get the
	// correct caller.
	level.Info(&l.swapLogger).Log(keyvals...)
	os.Exit(1)
	// never hit
	return nil
}

func NewLogger(w io.Writer) Logger {
	base := kitlog.NewJSONLogger(kitlog.NewSyncWriter(w))
	base = kitlog.With(base, "ts", kitlog.DefaultTimestampUTC)

	// The constant in log.Caller is fragile and must be set
	// appropriately based on the level of wrapping of the logger. If the
	// wrapping changes and this value becomes set incorrectly, TestCaller
	// should fail.
	base = kitlog.With(base, "caller", kitlog.Caller(7))

	l := &logger{
		baseLogger: base,
	}
	l.swapLogger.Swap(level.NewFilter(l.baseLogger, level.AllowInfo()))

	return l
}
