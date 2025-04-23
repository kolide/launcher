package tablehelpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"go.opentelemetry.io/otel/attribute"
)

// ExecOps is a type for functional arguments to Exec, which changes the behavior of the exec command.
// An example of this is to run the exec as a specific user instead of root.
type ExecOps func(*exec.Cmd) error

func WithDir(dir string) ExecOps {
	return func(cmd *exec.Cmd) error {
		cmd.Dir = dir
		return nil
	}
}

func WithAppendEnv(key, value string) ExecOps {
	return func(cmd *exec.Cmd) error {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		return nil
	}
}

// RunSimple is a wrapper over allowedcmd.AllowedCommand.
// It enforces a timeout, logs, observability. and returns the stdout as a byte slice.
// It discards stderr.
//
// This is not suitable for high performance work -- it allocates new buffers each time
// Use Run() for more control over the stdout and stderr streams.
func RunSimple(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, cmd allowedcmd.AllowedCommand, args []string, opts ...ExecOps) ([]byte, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	var stdout bytes.Buffer
	if err := Run(ctx, slogger, timeoutSeconds, cmd, args, &stdout, io.Discard, opts...); err != nil {
		return nil, err
	}

	return stdout.Bytes(), nil
}

// Run is a wrapper over allowedcmd.AllowedCommand. It enforces a timeout, logs, observability.
// Use RunSimple() for a simpler interface.
func Run(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, execCmd allowedcmd.AllowedCommand, args []string, stdout io.Writer, stderr io.Writer, opts ...ExecOps) error {
	ctx, span := observability.StartSpan(ctx)
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
	span.SetAttributes(attribute.StringSlice("exec.args", cmd.Args))

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	slogger.Log(ctx, slog.LevelDebug,
		"execing",
		"cmd", cmd.String(),
		"args", cmd.Args,
		"timeout", timeoutSeconds,
	)

	switch err := cmd.Run(); {
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return fmt.Errorf("could not find %s to run: %w", cmd.Path, err)
	case ctx.Err() != nil:
		// ctx.Err() should only be set if the context is canceled or done
		observability.SetError(span, ctx.Err())
		return fmt.Errorf("context canceled during exec '%s': %w", cmd.String(), ctx.Err())
	default:
		observability.SetError(span, err)
		return fmt.Errorf("exec '%s': %w", cmd.String(), err)
	}
}
