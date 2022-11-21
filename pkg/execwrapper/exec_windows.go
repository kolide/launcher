//go:build windows
// +build windows

package execwrapper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

func Exec(ctx context.Context, argv0 string, argv []string, envv []string, in *os.File) error {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	cmd := exec.CommandContext(ctx, argv0, argv[1:]...)
	cmd.Env = envv

	cmd.Stdin = in
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	level.Debug(logger).Log(
		"msg", "preparing to run command",
		"cmd", strings.Join(cmd.Args, " "),
	)

	// Now run it. This is faking exec, we need to distinguish
	// between a failure to execute, and a failure in in the called program.
	// I think https://github.com/golang/go/issues/26539 adds this functionality.
	err := cmd.Run()

	if cmd.ProcessState.ExitCode() == -1 {
		if err == nil {
			return fmt.Errorf("Unknown error trying to exec %s (and nil err)", strings.Join(cmd.Args, " "))
		}
		return fmt.Errorf("Unknown error trying to exec %s: %w", strings.Join(cmd.Args, " "), err)
	}

	if err != nil {
		level.Info(logger).Log(
			"msg", "got error on exec",
			"err", err,
		)
	}
	os.Exit(cmd.ProcessState.ExitCode())
	return errors.New("Exec shouldn't have gotten here.")

}
