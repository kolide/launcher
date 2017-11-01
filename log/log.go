package log

import (
	"io"
	"os"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type Logger struct {
	// baseLogger stores the un-leveled logger. Writes should not be done
	// directly to this logger.
	baseLogger kitlog.Logger
	// swapLogger stores the leveled logger, using go-kit's SwapLogger to
	// ensure that updates to the logger are atomic and avoid race
	// conditions. All logging should be done through swapLogger.
	swapLogger kitlog.SwapLogger
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
	level.Info(&l.swapLogger).Log(
		"msg", "allowed log level set",
		"allowed_level", name,
	)
}

// Log will log with level: unknown (filtered as if it were info). Prefer using
// an explicitly leveled log.Debug or log.Info.
func (l *Logger) Log(keyvals ...interface{}) error {
	return l.swapLogger.Log(keyvals...)
}

// Fatal will log with level: fatal and exit with a status 1
func (l *Logger) Fatal(keyvals ...interface{}) error {
	// Call this directly instead of using l.Info so that we get the
	// correct caller.
	level.Info(&l.swapLogger).Log(keyvals...)
	os.Exit(1)
	// never hit
	return nil
}

func NewLogger(w io.Writer) *Logger {
	base := kitlog.NewJSONLogger(kitlog.NewSyncWriter(w))
	base = kitlog.With(base, "ts", kitlog.DefaultTimestampUTC)
	base = kitlog.With(base, "component", "launcher")
	base = level.NewInjector(base, level.InfoValue())

	// The constant in log.Caller is fragile and must be set
	// appropriately based on the level of wrapping of the logger. If the
	// wrapping changes and this value becomes set incorrectly, TestCaller
	// should fail.
	base = kitlog.With(base, "caller", kitlog.Caller(7))

	l := &Logger{
		baseLogger: base,
	}
	l.swapLogger.Swap(level.NewFilter(l.baseLogger, level.AllowInfo()))

	return l
}

// OsqueryLogAdapater creates an io.Writer implementation useful for attaching
// to the osquery stdout/stderr
type OsqueryLogAdapter struct {
	kitlog.Logger
}

func (l *OsqueryLogAdapter) Write(p []byte) (int, error) {
	if err := l.Logger.Log("msg", string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
