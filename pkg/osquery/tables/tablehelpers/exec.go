package tablehelpers

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

// Exec is a wrapper over exec.CommandContext. It does a couple of
// additional things to help with table usage:
// 1. It enforces a timeout.
// 2. Second, it accepts an array of possible binaries locations, and if something is not
//    found, it will go down the list.
// 3. It moves the stderr into the return error, if needed.
//
// This is not suitable for high performance work -- it allocates new buffers each time.
func Exec(ctx context.Context, logger log.Logger, timeoutSeconds int, possibleBins []string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	for _, bin := range possibleBins {
		stdout.Reset()
		stderr.Reset()

		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

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
			return nil, errors.Wrapf(err, "exec '%s'. Got: '%s'", cmd.String(), string(stderr.Bytes()))
		}

	}
	// Getting here means no binary was found
	return nil, errors.Wrapf(os.ErrNotExist, "No binary found in specified paths: %v", possibleBins)
}
