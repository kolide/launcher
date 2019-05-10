// +build windows

// Authenticode is a light wrapper around signing code under windows.
//
// See
//
// https://docs.microsoft.com/en-us/dotnet/framework/tools/signtool-exe

package authenticode

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type digestAlgo string

const (
	SHA1   digestAlgo = "sha1"
	SHA256            = "sha256"
)

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
	// _However_ it's not clear this is supported for MSIs, which maybe only have a single
	//
	// References:
	// https://knowledge.digicert.com/generalinformation/INFO2274.html
	if strings.HasSuffix(file, ".msi") {
		if err := so.signtoolSign(ctx, file, true, SHA1); err != nil {
			return errors.Wrap(err, "signing exe with sha1")
		}
	} else {
		if err := so.signtoolSign(ctx, file, true, SHA1); err != nil {
			return errors.Wrap(err, "signing file with 0:sha1")
		}
		if err := so.signtoolSign(ctx, file, false, SHA256); err != nil {
			return errors.Wrap(err, "signing file with 1:sha256")
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

// constructSigntoolArgs returns an array of signtool.exe args based
// on whether this is the first or subsequent signature, and what the
// algorithm is.
//
// This is _very_ fragile. Not everthing you think will work does
func (so *signtoolOptions) signtoolSign(ctx context.Context, file string, firstSig bool, algo digestAlgo) error {
	ctx, span := trace.StartSpan(ctx, fmt.Sprintf("algo %s", algo))
	defer span.End()

	args := []string{
		"sign",
		"/fd", string(algo),
	}

	if !firstSig {
		args = append(args, "/as")
	}

	switch algo {
	case SHA1:
		args = append(args, "/t", so.timestampServer)
	case SHA256:
		args = append(args, "/td", "sha256", "/tr", so.rfc3161Server)
	}

	if so.extraArgs != nil {
		args = append(args, so.extraArgs...)
	}

	args = append(args, file)

	if _, _, err := so.execOut(ctx, so.signtoolPath, args...); err != nil {
		return errors.Wrap(err, "calling signtool")
	}
	return nil
}
