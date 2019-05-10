package authenticode

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// signtoolOptions are the options for how we call signtool.exe. These
// are *not* the tool options, but instead our own representation of
// the arguments.w
type signtoolOptions struct {
	extraArgs       []string
	subjectName     string // If present, use this as the `/n` argument
	skipValidation  bool
	signtoolPath    string
	timestampServer string
	rfc3161Server   string

	execCC func(context.Context, string, ...string) *exec.Cmd // Allows test overrides

}

type SigntoolOpt func(*signtoolOptions)

func SkipValidation() SigntoolOpt {
	return func(so *signtoolOptions) {
		so.skipValidation = true
	}
}

// WithExtraArgs set additional arguments for signtool. Common ones may be {`\n`, "subject name"}
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
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}
