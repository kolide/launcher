// +build windows

// Authenticode is a light wrapper around signing code under windows.
//
// See
//
// https://docs.microsoft.com/en-us/dotnet/framework/tools/signtool-exe

package authenticode

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

func Sign(ctx context.Context, file string, opts ...SigntoolOpt) error {
	so := &signtoolOptions{
		signtoolPath: "signtool.exe",
		execCC:       exec.CommandContext,
	}

	for _, opt := range opts {
		opt(so)
	}

	// signtool.exe can be called multiple times to apply multiple
	// signatures. _But_ it uses different arguments for the subsequent
	// signatures. So, multiple calls.
	// Some info at https://knowledge.digicert.com/generalinformation/INFO2274.html
	{
		args := []string{
			"sign",
			"/fd", "sha1",
			"/t", "http://timestamp.verisign.com/scripts/timstamp.dll",
			"/v",
		}
		if so.subjectName != "" {
			args = append(args, "/n", so.subjectName)
		}

		if _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
			return errors.Wrap(err, "calling signtool")
		}
	}
	{

		args = []string{
			"sign",
			"/as",
			"/fd", "sha256",
			"/tr", "http://sha256timestamp.ws.symantec.com/sha256/timestamp",
			"/td", "sha256",
			"/v",
		}

		if so.subjectName != "" {
			args = append(args, "/n", so.subjectName)
		}
		if _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
			return errors.Wrap(err, "calling signtool")
		}
	}

	fmt.Println("seph")

	// Verify!
	// FIXME
	return nil
}

func (so *signtoolOptions) execOut(ctx context.Context, argv0, args ...string) (string, error) {
	logger := ctxlog.FromContext(ctx)

	cmd := so.execCC(ctx, argv0, args...)

	fmt.Println(strings.Join(cmd.Args, " "))

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", strings.Join(cmd.Args, " "),
	)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), nil
}
