package cmdwrapper

// Package cmdwrapper is a simple wrapper around exec.Command. It
// exists to provide a unified place to keep `runas` code, as well as
// enforcing execution timeouts.

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type execOptions struct {
	timeout       time.Duration // Set an execution timeout. (default: 5s)
	runAsUser     string        // What user to run as (default: whatever launcher is running as)
	skipUserCheck bool
}

type Option func(*execOptions)

// WithTimeout sets the execution timeout
func WithTimeout(t time.Duration) Option {
	return func(eo *execOptions) {
		eo.timeout = t
	}
}

// RunAsUid sets the uid to execute as. Some platforms may require
// root or admin privledges to uses this.
//
// Normal usage detects when your target uid matches your effective
// uid and *does not* attempt to change permissions unless needed.
func RunAsUser(u string) Option {
	return func(eo *execOptions) {
		eo.runAsUser = u
	}
}

// AlwaysRunAs disables the effective user id check. See `RunAs`
func AlwaysRunAsUser() Option {
	return func(eo *execOptions) {
		eo.skipUserCheck = true
	}
}

// NewExec returns a configured exec.
func New(ctx context.Context, arg0 string, args []string, opts ...Option) (*exec.Cmd, context.CancelFunc, error) {
	execOptions := &execOptions{
		timeout: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(execOptions)
	}

	ctx, cancel := context.WithTimeout(ctx, execOptions.timeout)

	cmd := exec.CommandContext(ctx, arg0, args...)

	if execOptions.runAsUser != "" {
		if err := execOptions.setRunAsUser(cmd); err != nil {
			return nil, cancel, errors.Wrap(err, "set runas")
		}
	}

	return cmd, cancel, nil
}

func RunCombined(ctx context.Context, arg0 string, args []string, opts ...Option) (string, error) {
	cmd, cancel, err := New(ctx, arg0, args, opts...)
	defer cancel()
	if err != nil {
		return "", err
	}

	output := new(bytes.Buffer)
	cmd.Stdout = output
	cmd.Stderr = output

	cmdErr := cmd.Run()
	return strings.TrimSpace(output.String()), cmdErr
}

func Run(ctx context.Context, arg0 string, args []string, opts ...Option) (string, string, error) {
	cmd, cancel, err := New(ctx, arg0, args, opts...)
	defer cancel()
	if err != nil {
		return "", "", err
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	cmdErr := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), cmdErr
}
