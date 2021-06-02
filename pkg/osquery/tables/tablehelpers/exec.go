package tablehelpers

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

func Exec(ctx context.Context, logger log.Logger, bin string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", cmd.String(),
	)

	if err := cmd.Run(); err != nil {
		level.Info(logger).Log(
			"msg", "Error execing",
			"cmd", cmd.String(),
			"stderr", string(stderr.Bytes()),
		)
		return nil, errors.Wrapf(err, "calling %s. Got: %s", bin, string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}
