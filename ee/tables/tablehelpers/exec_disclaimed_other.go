//go:build !darwin
// +build !darwin

package tablehelpers

import (
	"context"
	"io"
	"log/slog"

	"github.com/kolide/launcher/ee/allowedcmd"
)

// only darwin builds have an API for disclaiming currently, default to our standard run functionality, allowing overrides for darwin where needed
func RunDisclaimed(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, execCmd allowedcmd.AllowedCommand, args []string, stdout io.Writer, stderr io.Writer, opts ...ExecOps) error {
	return Run(ctx, slogger, timeoutSeconds, execCmd, args, stdout, stderr)
}
