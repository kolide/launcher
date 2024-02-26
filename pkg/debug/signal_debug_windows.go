//go:build windows
// +build windows

package debug

import "log/slog"

func AttachDebugHandler(addrPath string, slogger *slog.Logger) {
	// TODO: noop for now
}
