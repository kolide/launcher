//go:build darwin
// +build darwin

package tablehelpers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel/attribute"
)

func RunDisclaimed(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, execCmd allowedcmd.AllowedCommand, args []string, stdout io.Writer, stderr io.Writer, opts ...ExecOps) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd, err := execCmd(ctx, args...)
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}

	for _, opt := range opts {
		if err := opt(cmd.Cmd); err != nil {
			return fmt.Errorf("applying option: %w", err)
		}
	}

	span.SetAttributes(attribute.String("exec.path", cmd.Path))
	span.SetAttributes(attribute.String("exec.binary", filepath.Base(cmd.Path)))
	span.SetAttributes(attribute.StringSlice("exec.args", args))

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	slogger.Log(ctx, slog.LevelDebug,
		"execing",
		"cmd", cmd.String(),
		"args", args,
		"timeout", timeoutSeconds,
	)

	switch err := cmd.Run(); {
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return fmt.Errorf("could not find %s to run: %w", cmd.Path, err)
	case ctx.Err() != nil:
		// ctx.Err() should only be set if the context is canceled or done
		traces.SetError(span, ctx.Err())
		return fmt.Errorf("context canceled during exec '%s': %w", cmd.String(), ctx.Err())
	default:
		traces.SetError(span, err)
		return fmt.Errorf("exec '%s': %w", cmd.String(), err)
	}
}
