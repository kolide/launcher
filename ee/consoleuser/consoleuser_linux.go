//go:build linux
// +build linux

package consoleuser

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type listSessionsResult []struct {
	Session string `json:"session"`
	UID     int    `json:"uid"`
	Seat    string `json:"seat"`
}

func CurrentUids(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "loginctl", "list-sessions", "--no-legend", "--no-pager", "--output=json").Output()
	if err != nil {
		return nil, fmt.Errorf("loginctl list-sessions: %w", err)
	}

	// unmarshall json output into listSessionsResult
	var sessions listSessionsResult
	if err := json.Unmarshal(output, &sessions); err != nil {
		return nil, fmt.Errorf("loginctl list-sessions unmarshall json output: %w", err)
	}

	var uids []string
	for _, s := range sessions {
		// generally human users start at 1000 on linux
		if s.UID < 1000 {
			continue
		}

		remoteVal, err := sessionProperty(ctx, s.Session, "Remote")
		if err != nil {
			return nil, err
		}

		// don't count ssh users as console users
		// ssh: remote=yes
		// local: remote=no
		// rdp: remote=no
		if remoteVal == "yes" {
			continue
		}

		activeVal, err = sessionProperty(ctx, s.Session, "Active")
		if err != nil {
			return nil, err
		}

		// don't count inactive users as console users
		if activeVal == "no" {
			continue
		}

		uids = append(uids, fmt.Sprintf("%d", s.UID))
	}

	return uids, nil
}

func sessionProperty(ctx context.Context, sessionId, property string) (string, error){
	output, err := exec.CommandContext(ctx,
		"loginctl",
		"show-session", sessionId,
		"--value", fmt.Sprintf("--property=%s", property)
	).Output()

	if err != nil {
		return "", fmt.Errorf("loginctl show-session (for sessionId %s): %w", sessionId, err)
	}

	return strings.Trim(string(output), "\n"), nil
}
