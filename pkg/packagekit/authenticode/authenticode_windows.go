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
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// Sign uses signtool to add authenticode signatures. It supports
// optional arguments to allow cert specification
func Sign(ctx context.Context, file string, opts ...SigntoolOpt) error {
	ctx, span := trace.StartSpan(ctx, "authenticode.Sign")
	defer span.End()

	logger := log.With(ctxlog.FromContext(ctx), "caller", "authenticode.Sign")

	level.Debug(logger).Log(
		"msg", "signing file",
		"file", file,
	)

	so := &signtoolOptions{
		signtoolPath:    "signtool.exe",
		timestampServer: "http://timestamp.verisign.com/scripts/timstamp.dll",
		rfc3161Server:   "http://sha256timestamp.ws.symantec.com/sha256/timestamp",
		execCC:          exec.CommandContext,
	}

	for _, opt := range opts {
		opt(so)
	}

	// signtool.exe can be called multiple times to apply multiple
	// signatures. _But_ it uses different arguments for the subsequent
	// signatures. So, multiple calls.
	//
	// _However_ it's not clear this is supported for MSIs, which maybe
	// only have a single slot for signing.
	//
	// References:
	// https://knowledge.digicert.com/generalinformation/INFO2274.html
	if strings.HasSuffix(file, ".msi") {
		if err := so.signtoolSign(ctx, file, "/fd", "sha1", "/t", so.timestampServer); err != nil {
			return errors.Wrap(err, "signing msi with sha1")
		}
	} else {
		if err := so.signtoolSign(ctx, file, "/fd", "sha1", "/t", so.timestampServer); err != nil {
			return errors.Wrap(err, "signing msi with sha1")
		}

		if err := so.signtoolSign(ctx, file, "/as", "/fd", "sha256", "/td", "sha256", "/tr", so.rfc3161Server); err != nil {
			return errors.Wrap(err, "signing msi with sha1")
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

// signtoolSign appends some arguments and execs
func (so *signtoolOptions) signtoolSign(ctx context.Context, file string, args ...string) error {
	ctx, span := trace.StartSpan(ctx, "signtoolSign")
	defer span.End()

	if so.extraArgs != nil {
		args = append(args, so.extraArgs...)
	}

	args = append(args, file)

	if _, _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
		return errors.Wrap(err, "calling signtool")
	}
	return nil
}
