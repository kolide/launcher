//go:build !windows
// +build !windows

package presencedetection

import (
	"context"
	"log/slog"
)

func WindowsHello(_ context.Context, _ *slog.Logger) {
	return
}
