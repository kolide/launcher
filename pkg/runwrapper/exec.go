package runwrapper

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

type execOptions struct {
	timeout        time.Duration // Set an execution timeout. (default: 5s)
	runAsUid       string        // What user to run as (default: whatever launcher is running as)
	runAsGid       string
	alwaysRunAsUid bool
	alwaysRunAsGid bool

	simulated       bool
	simulatedStdout string
	simulatedStderr string
	simulatedErr    error
}

type Option func(*execOptions)

// WithSimulatedResponse returns the given ExecReturn instead of
// executing. Used for testing output parsers.
func WithSimulatedResponse(stdout, stderr string, err error) Option {
	return func(eo *execOptions) {
		eo.simulated = true
		eo.simulatedStdout = stdout
		eo.simulatedStderr = stderr
		eo.simulatedErr = err
	}
}

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
// uid and *does not* attempt to escalate permissions. (This bypass is
// to allow easier usage when testing as non-root)
func RunAsUid(uid string) Option {
	return func(eo *execOptions) {
		eo.runAsUid = uid
	}
}

// AlwaysRunAs disables the effective user id check. See `RunAs`
func AlwaysRunAsUid() Option {
	return func(eo *execOptions) {
		eo.alwaysRunAsUid = true
	}
}

// RunAsGid sets the gid to execute as. Some platforms may
// require root or admin privledges to uses this.
//
// Normal usage detects
// when your target uid matches your effective uid and *does not*
// attempt to escalate permissions. (This bypass is to allow easier
// usage when testing as non-root)
func RunAsGid(gid string) Option {
	return func(eo *execOptions) {
		eo.runAsGid = gid
	}
}

// AlwaysRunAs disables the effective user id check. See `RunAs`
func AlwaysRunAsGid() Option {
	return func(eo *execOptions) {
		eo.alwaysRunAsGid = true
	}
}

// Exec is a wrapper around exec. It's here to ensure that tables that
// exec use a consistent format, to provide a simple set of
// options. It includes simple functions for testing and enforcing
// timeouts.
func Exec(ctx context.Context, arg0 string, args []string, opts ...Option) (string, string, error) {
	execOptions := &execOptions{
		timeout: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(execOptions)
	}

	ctx, cancel := context.WithTimeout(ctx, execOptions.timeout)
	defer cancel()

	logger := ctxlog.FromContext(ctx)

	if execOptions.simulated {
		level.Debug(logger).Log("msg", "Returning simulated response")
		return execOptions.simulatedStdout, execOptions.simulatedStderr, execOptions.simulatedErr
	}

	level.Debug(logger).Log(
		"msg", "launcher exec",
		"cmd", arg0,
		"args", fmt.Sprintf("%v+", args),
	)

	cmd := exec.CommandContext(ctx, arg0, args...)

	if err := execOptions.setRunAs(cmd); err != nil {
		return "", "", errors.Wrap(err, "set runas")
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	err := cmd.Run()

	if err != nil {
		level.Info(logger).Log(
			"msg", "exec failed. Got error",
			"cmd", arg0,
			"args", fmt.Sprintf("%v+", args),
			"stdout", strings.TrimSpace(stdout.String()),
			"stderr", strings.TrimSpace(stderr.String()),
			"err", err,
		)
		return "", "", errors.Wrapf(err, "run command %s %v, stderr=%s", arg0, args, stderr)
	}

	return strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()), nil
}
