//go:build darwin
// +build darwin

package tablehelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/user"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

// ExecOsqueryLaunchctl runs osquery under launchctl, in a user context.
func ExecOsqueryLaunchctl(ctx context.Context, timeoutSeconds int, username string, osqueryPath string, query string) ([]byte, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	targetUser, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("looking up username %s: %w", username, err)
	}

	args := []string{
		"asuser",
		targetUser.Uid,
		osqueryPath,
		"--config_path", "/dev/null",
		"--disable_events",
		"--disable_database",
		"--disable_audit",
		"--ephemeral",
		"-S",
		"--json",
		query,
	}

	dir, err := agent.MkdirTemp("osq-launchctl")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return nil, fmt.Errorf("chmod: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, multislogger.NewNopLogger(), timeoutSeconds, allowedcmd.Launchctl, args, &stdout, &stderr, WithDir(dir)); err != nil {
		return nil, fmt.Errorf("running osquery. Got: '%s': %w", stderr.String(), err)
	}

	return stdout.Bytes(), nil
}

func ExecOsqueryLaunchctlParsed(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, username string, osqueryPath string, query string) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	outBytes, err := ExecOsqueryLaunchctl(ctx, timeoutSeconds, username, osqueryPath, query)
	if err != nil {
		return nil, err
	}

	var osqueryResults []map[string]string

	if err := json.Unmarshal(outBytes, &osqueryResults); err != nil {
		slogger.Log(ctx, slog.LevelInfo,
			"error unmarshalling json",
			"err", err,
			"stdout", string(outBytes),
		)
		return nil, fmt.Errorf("unmarshalling json: %w", err)
	}

	return osqueryResults, nil
}
