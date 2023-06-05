package authenticode

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

// signtoolOptions are the options for how we call signtool.exe. These
// are *not* the tool options, but instead our own representation of
// the arguments.w
type signtoolOptions struct {
	extraArgs []string
	// If present, use this as the `/n` argument
	subjectName     string // nolint:unused
	skipValidation  bool
	signtoolPath    string
	timestampServer string // nolint:unused
	rfc3161Server   string // nolint:unused

	// Allows test overrides
	execCC func(context.Context, string, ...string) *exec.Cmd // nolint:unused

}

type SigntoolOpt func(*signtoolOptions)

func SkipValidation() SigntoolOpt {
	return func(so *signtoolOptions) {
		so.skipValidation = true
	}
}

// WithExtraArgs set additional arguments for signtool. Common ones
// may be {`\n`, "subject name"}
func WithExtraArgs(args []string) SigntoolOpt {
	return func(so *signtoolOptions) {
		so.extraArgs = args
	}
}

func WithSigntoolPath(path string) SigntoolOpt {
	return func(so *signtoolOptions) {
		so.signtoolPath = path
	}
}

// nolint:unused
func (so *signtoolOptions) execOut(ctx context.Context, argv0 string, args ...string) (string, string, error) {
	logger := ctxlog.FromContext(ctx)

	cmd := so.execCC(ctx, argv0, args...)

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", strings.Join(cmd.Args, " "),
	)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), fmt.Errorf("run command %s %v, stderr=%s: %w", argv0, args, stderr, err)
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}
