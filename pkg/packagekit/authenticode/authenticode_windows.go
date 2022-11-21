//go:build windows
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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"

	"go.opencensus.io/trace"
)

const (
	maxRetries         = 5
	retryDelay         = 30 * time.Second
	timeoutErrorString = "The specified timestamp server either could not be reached"
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
		if err := so.signtoolSign(ctx, file, "/ph", "/fd", "sha256", "/td", "sha256", "/tr", so.rfc3161Server); err != nil {
			return fmt.Errorf("signing msi with sha256: %w", err)
		}
	} else {
		if err := so.signtoolSign(ctx, file, "/ph", "/fd", "sha1", "/t", so.timestampServer); err != nil {
			return fmt.Errorf("signing file with sha1: %w", err)
		}

		if err := so.signtoolSign(ctx, file, "/as", "/ph", "/fd", "sha256", "/td", "sha256", "/tr", so.rfc3161Server); err != nil {
			return fmt.Errorf("signing file with sha256: %w", err)
		}
	}

	if so.skipValidation {
		return nil
	}

	_, _, err := so.execOut(ctx, so.signtoolPath, "verify", "/pa", "/v", file)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	return nil
}

// signtoolSign appends some arguments and execs
func (so *signtoolOptions) signtoolSign(ctx context.Context, file string, args ...string) error {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	ctx, span := trace.StartSpan(ctx, "signtoolSign")
	defer span.End()

	args = append([]string{"sign"}, args...)

	if so.extraArgs != nil {
		args = append(args, so.extraArgs...)
	}

	args = append(args, file)

	// The timestamp servers timeout sometimes. So we
	// implement a retry logic here.
	attempt := 0
	for {
		attempt = attempt + 1
		_, stderr, err := so.execOut(ctx, so.signtoolPath, args...)
		if err == nil {
			return nil
		}

		if attempt > maxRetries {
			return fmt.Errorf("Reached max number of retries. %d is too many: %w", attempt, err)
		}

		if !strings.Contains(stderr, timeoutErrorString) {
			level.Debug(logger).Log(
				"msg", "signtool got a retryable error. Sleeping and will try again",
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(retryDelay)
			continue
		}

		// Fallthrough to catch errors unrelated to timeouts
		return fmt.Errorf("calling signtool: %w", err)
	}
}
