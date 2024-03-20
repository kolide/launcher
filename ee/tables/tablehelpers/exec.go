package tablehelpers

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel/attribute"
)

type execOps func(*exec.Cmd) error

func WithUid(uid string) execOps {
	return func(cmd *exec.Cmd) error {
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("getting current user: %w", err)
		}

		runningUser, err := user.LookupId(uid)
		if err != nil {
			return fmt.Errorf("looking up user with uid %s: %w", uid, err)
		}

		if currentUser.Uid != "0" && currentUser.Uid != runningUser.Uid {
			return fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
		}

		runningUserUid, err := strconv.ParseUint(runningUser.Uid, 10, 32)
		if err != nil {
			return fmt.Errorf("converting uid %s to int: %w", runningUser.Uid, err)
		}

		runningUserGid, err := strconv.ParseUint(runningUser.Gid, 10, 32)
		if err != nil {
			return fmt.Errorf("converting gid %s to int: %w", runningUser.Gid, err)
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(runningUserUid),
				Gid: uint32(runningUserGid),
			},
		}

		return nil
	}
}

// Exec is a wrapper over exec.CommandContext. It does a couple of
// additional things to help with table usage:
//  1. It enforces a timeout.
//  2. Second, it accepts an array of possible binaries locations, and if something is not
//     found, it will go down the list.
//  3. It moves the stderr into the return error, if needed.
//
// This is not suitable for high performance work -- it allocates new buffers each time.
//
// `possibleBins` can be either a list of command names, or a list of paths to commands.
// Where reasonable, `possibleBins` should be command names only, so that we can perform
// lookup against PATH.
func Exec(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, execCmd allowedcmd.AllowedCommand, args []string, includeStderr bool, opts ...execOps) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := execCmd(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("creating command: %w", err)
	}

	for _, opt := range opts {
		if err := opt(cmd); err != nil {
			return nil, err
		}
	}

	span.SetAttributes(attribute.String("exec.path", cmd.Path))
	span.SetAttributes(attribute.String("exec.binary", filepath.Base(cmd.Path)))
	span.SetAttributes(attribute.StringSlice("exec.args", args))

	cmd.Stdout = &stdout
	if includeStderr {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}

	slogger.Log(ctx, slog.LevelDebug,
		"execing",
		"cmd", cmd.String(),
	)

	switch err := cmd.Run(); {
	case err == nil:
		return stdout.Bytes(), nil
	case os.IsNotExist(err):
		return nil, fmt.Errorf("could not find %s to run: %w", cmd.Path, err)
	case ctx.Err() != nil:
		// ctx.Err() should only be set if the context is canceled or done
		traces.SetError(span, ctx.Err())
		return nil, fmt.Errorf("context canceled during exec '%s'. Got: '%s': %w", cmd.String(), stderr.String(), ctx.Err())
	default:
		traces.SetError(span, err)
		return nil, fmt.Errorf("exec '%s'. Got: '%s': %w", cmd.String(), stderr.String(), err)
	}
}
