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

		output, err := exec.CommandContext(ctx,
			"loginctl",
			"show-session", s.Session,
			"--property=Remote",
			"--property=Active",
		).Output()

		if err != nil {
			return nil, fmt.Errorf("loginctl show-session (for sessionId %s): %w", s.Session, err)
		}

		// to make remote session behave like local session and include systray icons on ubuntu 22.04
		// had to create a ~/.xsessionrc file with the following content:
		// export GNOME_SHELL_SESSION_MODE=ubuntu
		// export XDG_CURRENT_DESKTOP=ubuntu:GNOME
		// export XDG_CONFIG_DIRS=/etc/xdg/xdg-ubuntu:/etc/xdg

		// ssh: remote=yes
		// local: remote=no
		// rdp: remote=no
		if strings.Contains(string(output), "Remote=no") && strings.Contains(string(output), "Active=yes") {
			uids = append(uids, fmt.Sprintf("%d", s.UID))
		}
	}

	return uids, nil
}
