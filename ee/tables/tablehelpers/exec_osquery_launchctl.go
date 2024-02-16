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
	"time"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
)

// ExecOsqueryLaunchctl runs osquery under launchctl, in a user context.
func ExecOsqueryLaunchctl(ctx context.Context, timeoutSeconds int, username string, osqueryPath string, query string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	targetUser, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("looking up username %s: %w", username, err)
	}

	cmd, err := allowedcmd.Launchctl(ctx,
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
	)
	if err != nil {
		return nil, fmt.Errorf("creating launchctl command: %w", err)
	}

	dir, err := agent.MkdirTemp("osq-launchctl")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return nil, fmt.Errorf("chmod: %w", err)
	}

	cmd.Dir = dir

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running osquery. Got: '%s': %w", stderr.String(), err)
	}

	return stdout.Bytes(), nil

}

func ExecOsqueryLaunchctlParsed(ctx context.Context, slogger *slog.Logger, timeoutSeconds int, username string, osqueryPath string, query string) ([]map[string]string, error) {
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
