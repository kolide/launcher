//go:build linux
// +build linux

package consoleuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/allowedcmd"
)

type listSessionsResult []struct {
	Session string `json:"session"`
	UID     int    `json:"uid"`
	Seat    string `json:"seat"`
}

func CurrentUids(ctx context.Context, logger log.Logger) ([]string, error) {
	logger = log.With(logger, "method", "CurrentUids")

	cmd, err := allowedcmd.Loginctl(ctx, "list-sessions", "--no-legend", "--no-pager", "--output=json")
	if err != nil {
		level.Error(logger).Log("msg", "creating loginctl command", "err", err)
		return nil, fmt.Errorf("creating loginctl command: %w", err)
	}
	output, err := cmd.Output()
	if err != nil {
		level.Error(logger).Log("msg", "listing sessions", "err", err)
		return nil, fmt.Errorf("loginctl list-sessions: %w", err)
	}

	level.Info(logger).Log("msg", "listed sessions", "output", string(output))

	// unmarshall json output into listSessionsResult
	var sessions listSessionsResult
	if err := json.Unmarshal(output, &sessions); err != nil {
		level.Error(logger).Log("msg", "loginctl list-sessions unmarshall json output", "err", err)
		return nil, fmt.Errorf("loginctl list-sessions unmarshall json output: %w", err)
	}

	var uids []string
	for _, s := range sessions {
		// generally human users start at 1000 on linux
		if s.UID < 1000 {
			level.Info(logger).Log("msg", "user id under 1000", "uid", s.UID)
			continue
		}

		cmd, err := allowedcmd.Loginctl(ctx,
			"show-session", s.Session,
			"--property=Remote",
			"--property=Active",
		)
		if err != nil {
			level.Error(logger).Log("msg", "creating loginctl command", "err", err)
			return nil, fmt.Errorf("creating loginctl command: %w", err)
		}

		output, err := cmd.Output()
		if err != nil {
			level.Error(logger).Log("msg", "showing session", "err", err, "session_id", s.Session)
			return nil, fmt.Errorf("loginctl show-session (for sessionId %s): %w", s.Session, err)
		}

		level.Info(logger).Log("msg", "loginctl show-session output", "uid", s.UID, "session_id", s.Session, "output", string(output))

		// to make remote session behave like local session and include systray icons on ubuntu 22.04
		// had to create a ~/.xsessionrc file with the following content:
		// export GNOME_SHELL_SESSION_MODE=ubuntu
		// export XDG_CURRENT_DESKTOP=ubuntu:GNOME
		// export XDG_CONFIG_DIRS=/etc/xdg/xdg-ubuntu:/etc/xdg

		// ssh: remote=yes
		// local: remote=no
		// rdp: remote=no
		if strings.Contains(string(output), "Remote=no") && strings.Contains(string(output), "Active=yes") {
			level.Info(logger).Log("msg", "adding user", "uid", s.UID)
			uids = append(uids, fmt.Sprintf("%d", s.UID))
		}
	}

	return uids, nil
}
