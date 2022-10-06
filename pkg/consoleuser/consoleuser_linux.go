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

func CurrentUids(context context.Context) ([]string, error) {
	output, err := exec.CommandContext(context, "loginctl", "list-sessions", "--no-legend", "--no-pager", "--output=json").Output()
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
		// if there is no seat, this is not a graphical session
		if s.Seat == "" {
			continue
		}

		// get the active property of the session, this command does not respect the --output=json flag
		output, err := exec.CommandContext(context, "loginctl", "show-session", s.Session, "--value", "--property=Active").Output()
		if err != nil {
			return nil, fmt.Errorf("loginctl show-session: %w", err)
		}

		if strings.Trim(string(output), "\n") != "yes" {
			continue
		}

		uids = append(uids, fmt.Sprintf("%d", s.UID))
	}

	return uids, nil
}
