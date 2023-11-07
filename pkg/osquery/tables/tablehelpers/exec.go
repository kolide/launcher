package tablehelpers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/allowedpaths"
	"github.com/kolide/launcher/pkg/traces"
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
func Exec(ctx context.Context, logger log.Logger, timeoutSeconds int, possibleBins []string, args []string, includeStderr bool) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx,
		"possible_binaries", possibleBins,
		"args", args)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	for _, bin := range possibleBins {
		stdout.Reset()
		stderr.Reset()

		var cmd *exec.Cmd
		var err error
		// If we only have the binary name and not the path, try to perform lookup
		if filepath.Base(bin) == bin {
			cmd, err = allowedpaths.CommandContextWithLookup(ctx, bin, args...)
		} else {
			cmd, err = allowedpaths.CommandContextWithPath(ctx, bin, args...)
		}
		if err != nil {
			// Likely that binary was not found -- try the next
			continue
		}

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

		switch err := cmd.Run(); {
		case err == nil:
			return stdout.Bytes(), nil
		case os.IsNotExist(err):
			// try the next binary
			continue
		default:
			// an actual error
			traces.SetError(span, err)
			return nil, fmt.Errorf("exec '%s'. Got: '%s': %w", cmd.String(), string(stderr.Bytes()), err)
		}

	}
	// Getting here means no binary was found
	return nil, fmt.Errorf("No binary found in specified paths: %v: %w", possibleBins, os.ErrNotExist)
}
