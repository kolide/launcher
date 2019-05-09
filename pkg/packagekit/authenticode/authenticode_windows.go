// +build windows

// Authenticode is a light wrapper around signing code under windows.
//
// See
//
// https://docs.microsoft.com/en-us/dotnet/framework/tools/signtool-exe

package authenticode

import (
	"context"
	"os/exec"

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

		args = append(args, file)
		if _, _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
			return errors.Wrap(err, "calling signtool")
		}
	}
	{
		args := []string{
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

		args = append(args, file)
		if _, _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
			return errors.Wrap(err, "calling signtool")
		}
	}

	if so.skipValidation {
		return nil
	}

	_, _, err := so.execOut(ctx, so.signtoolPath, "verify", "/pa", "/v", file)
	if err != nil {
		return errors.Wrap(err, "verify")
	}

	return nil
}
