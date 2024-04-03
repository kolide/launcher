//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/kolide/launcher/pkg/log/eventlog"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"golang.org/x/sys/windows"
)

func systemSlogger() (*multislogger.MultiSlogger, io.Closer, error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		syslogger := defaultSystemSlogger()

		syslogger.Log(context.TODO(), slog.LevelDebug,
			"launcher running on windows without elevated permissions, using default stderr instead of eventlog",
		)

		return defaultSystemSlogger(), io.NopCloser(nil), nil
	}

	eventLogWriter, err := eventlog.NewWriter(serviceName)
	if err != nil {
		return nil, nil, fmt.Errorf("creating eventlog writer: %w", err)
	}

	systemSlogger := multislogger.New(slog.NewJSONHandler(eventLogWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return systemSlogger, eventLogWriter, nil
}
