//go:build windows
// +build windows

package multislogger

import (
	"context"
	"io"
	"log/slog"

	"github.com/kolide/launcher/pkg/log/eventlog"
	"golang.org/x/sys/windows"
)

const serviceName = "launcher"

func SystemSlogger() (*MultiSlogger, io.Closer, error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		syslogger := defaultSystemSlogger()

		syslogger.Log(context.TODO(), slog.LevelInfo,
			"launcher running on windows without elevated permissions, using default stderr instead of eventlog",
		)

		// Return a closer that stops multislogger resources
		return syslogger, closerFunc(func() error { syslogger.Stop(); return nil }), nil
	}

	eventLogWriter, err := eventlog.NewWriter(serviceName)
	if err != nil || eventLogWriter == nil {
		syslogger := defaultSystemSlogger()

		syslogger.Log(context.TODO(), slog.LevelError,
			"could not create eventlog writer, using default stderr instead of eventlog",
			"err", err,
		)

		// Return a closer that stops multislogger resources
		return syslogger, closerFunc(func() error { syslogger.Stop(); return nil }), nil
	}

	systemSlogger := New(slog.NewJSONHandler(eventLogWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Return a closer that first closes the eventlog writer, then stops the multislogger
	return systemSlogger, closerFunc(func() error {
		if eventLogWriter != nil {
			_ = eventLogWriter.Close()
		}
		systemSlogger.Stop()
		return nil
	}), nil
}
