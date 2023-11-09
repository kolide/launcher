//go:build linux
// +build linux

package consoleuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kolide/launcher/pkg/allowedcmd"
)

type listSessionsResult []struct {
	Session string `json:"session"`
	UID     int    `json:"uid"`
	Seat    string `json:"seat"`
}

func CurrentUids(ctx context.Context) ([]string, error) {
	cmd, err := allowedcmd.Loginctl(ctx, "list-sessions", "--no-legend", "--no-pager", "--output=json")
	if err != nil {
		return nil, fmt.Errorf("creating loginctl command: %w", err)
	}
	output, err := cmd.Output()
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

		// if there is no seat, this is not a graphical session
		if s.Seat == "" {
			continue
		}

		// get the active property of the session, this command does not respect the --output=json flag
		cmd, err := allowedcmd.Loginctl(ctx, "show-session", s.Session, "--value", "--property=Active")
		if err != nil {
			return nil, fmt.Errorf("creating loginctl command: %w", err)
		}
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("loginctl show-session (for uid %d): %w", s.UID, err)
		}

		if strings.Trim(string(output), "\n") != "yes" {
			continue
		}

		uids = append(uids, fmt.Sprintf("%d", s.UID))
	}

	return uids, nil
}
