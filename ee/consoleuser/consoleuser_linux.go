//go:build linux
// +build linux

package consoleuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
)

type listSessionsResult []struct {
	Session  string `json:"session"`
	UID      int    `json:"uid"`
	Username string `json:"user"`
	Seat     string `json:"seat"`
}

func CurrentUids(ctx context.Context) ([]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	sessions, err := listSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	var uids []string
	for _, s := range sessions {
		// generally human users start at 1000 on linux. 65534 is reserved for https://wiki.ubuntu.com/nobody,
		// which we don't want to count as a current user.
		if s.UID < 1000 || s.UID == 65534 || s.Username == "nobody" {
			continue
		}

		cmd, err := allowedcmd.Loginctl(ctx,
			"show-session", s.Session,
			"--property=Remote",
			"--property=Active",
		)
		if err != nil {
			return nil, fmt.Errorf("creating loginctl command: %w", err)
		}

		output, err := cmd.Output()
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

// listSessions execs `loginctl list-sessions` in order to retrieve the current list of sessions.
// Depending on the systemd version, we have to use different flags to output the results as JSON.
// We may want to attempt parsing the output regardless in the future -- see launcher #1522.
func listSessions(ctx context.Context) (listSessionsResult, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	var sessions listSessionsResult

	// Try with `--output=json` first, to support the more widely-used older versions of systemd
	legacyCmd, err := allowedcmd.Loginctl(ctx, "list-sessions", "--no-legend", "--no-pager", "--output=json")
	if err != nil {
		return nil, fmt.Errorf("creating loginctl command --no-legend --no-pager --output=json: %w", err)
	}
	legacyOut, err := legacyCmd.Output()
	if err == nil {
		// Newer versions of systemd ignore `--output=json` rather than returning an error, so we also
		// need to unmarshal the result to confirm we got expected output.
		if err := json.Unmarshal(legacyOut, &sessions); err == nil {
			return sessions, nil
		}
	}

	cmd, err := allowedcmd.Loginctl(ctx, "list-sessions", "--no-legend", "--no-pager", "--json=short")
	if err != nil {
		return nil, fmt.Errorf("loginctl list-sessions --no-legend --no-pager --json=short: %w", err)
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("loginctl list-sessions: %w", err)
	}
	if err := json.Unmarshal(output, &sessions); err != nil {
		return nil, fmt.Errorf("unmarshalling loginctl list-sessions output: %w", err)
	}

	return sessions, nil
}
