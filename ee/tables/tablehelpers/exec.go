package tablehelpers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel/attribute"
)

// Exec is a wrapper over exec.CommandContext. It does a couple of
// additional things to help with table usage:
//  1. It enforces a timeout.
//  2. Second, it accepts an array of possible binaries locations, and if something is not
//     found, it will go down the list.
//  3. It moves the stderr into the return error, if needed.
//
// This is not suitable for high performance work -- it allocates new buffers each time.
//
// `possibleBins` can be either a list of command names, or a list of paths to commands.
// Where reasonable, `possibleBins` should be command names only, so that we can perform
// lookup against PATH.
func Exec(ctx context.Context, logger log.Logger, timeoutSeconds int, execCmd allowedcmd.AllowedCommand, args []string, includeStderr bool) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := execCmd(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("creating command: %w", err)
	}

	span.SetAttributes(attribute.String("exec.path", cmd.Path))
	span.SetAttributes(attribute.String("exec.binary", filepath.Base(cmd.Path)))
	span.SetAttributes(attribute.StringSlice("exec.args", args))

	cmd.Stdout = &stdout
	if includeStderr {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", cmd.String(),
	)

	// FIXME:  log if the error is a timeout

	switch err := cmd.Run(); {
	case err == nil:
		return stdout.Bytes(), nil
	case os.IsNotExist(err):
		return nil, fmt.Errorf("could not find %s to run: %w", cmd.Path, err)
	case ctx.Err() != nil:
		// ctx.Err() should only be set if the context is canceled or done
		traces.SetError(span, ctx.Err())
		return nil, fmt.Errorf("context canceled during exec '%s'. Got: '%s': %w", cmd.String(), stderr.String(), ctx.Err())
	default:
		traces.SetError(span, err)
		return nil, fmt.Errorf("exec '%s'. Got: '%s': %w", cmd.String(), stderr.String(), err)
	}
}
