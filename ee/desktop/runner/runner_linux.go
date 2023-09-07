//go:build linux
// +build linux

package runner

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/shirou/gopsutil/v3/process"
)

const defaultDisplay = ":0"

func (r *DesktopUsersProcessesRunner) userEnvVars(ctx context.Context, uid string) map[string]string {
	envVars := make(map[string]string)

	uidInt, err := strconv.ParseInt(uid, 10, 32)
	if err != nil {
		level.Debug(r.logger).Log(
			"msg", "could not convert uid to int32",
			"err", err,
		)
		return envVars
	}

	// Get the user's session so we can get their display (needed for opening notification action URLs in browser)
	sessionOutput, err := exec.CommandContext(ctx, "loginctl", "show-user", uid, "--value", "--property=Sessions").Output()
	if err != nil {
		level.Debug(r.logger).Log(
			"msg", "could not get user session",
			"uid", uid,
			"err", err,
		)
		return envVars
	}

	sessions := strings.Trim(string(sessionOutput), "\n")
	if sessions == "" {
		return envVars
	}

	sessionList := strings.Split(sessions, " ")
	for _, session := range sessionList {
		// Figure out what type of graphical session the user has -- x11, wayland?
		typeOutput, err := exec.CommandContext(ctx, "loginctl", "show-session", session, "--value", "--property=Type").Output()
		if err != nil {
			level.Debug(r.logger).Log(
				"msg", "could not get session type",
				"uid", uid,
				"err", err,
			)
			continue
		}

		sessionType := strings.Trim(string(typeOutput), "\n")
		if sessionType == "x11" {
			envVars["DISPLAY"] = r.displayFromX11(ctx, session)
			break
		} else if sessionType == "wayland" {
			envVars["DISPLAY"] = r.displayFromXwayland(ctx, int32(uidInt))

			break
		} else {
			// Not a graphical session
			continue
		}
	}

	// For opening links with xdg-open, we need XDG_DATA_DIRS so that xdg-open can find the mimetype configuration
	// files to figure out what application to launch.

	// We take the default value according to https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html,
	// but also include the snapd directory due to an issue on Ubuntu 22.04 where the default
	// /usr/share/applications/mimeinfo.cache does not contain any applications installed via snap.
	envVars["XDG_DATA_DIRS"] = "/usr/local/share/:/usr/share/:/var/lib/snapd/desktop"

	return envVars
}

func (r *DesktopUsersProcessesRunner) displayFromX11(ctx context.Context, session string) string {
	// We can read $DISPLAY from the session properties
	xDisplayOutput, err := exec.CommandContext(ctx, "loginctl", "show-session", session, "--value", "--property=Display").Output()
	if err != nil {
		level.Debug(r.logger).Log(
			"msg", "could not get Display from user session",
			"err", err,
		)
		return defaultDisplay
	}

	display := strings.Trim(string(xDisplayOutput), "\n")
	if display == "" {
		return defaultDisplay
	}

	return display
}

func (r *DesktopUsersProcessesRunner) displayFromXwayland(ctx context.Context, uid int32) string {
	//For wayland, DISPLAY is not included in loginctl show-session output -- in GNOME,
	// Mutter spawns Xwayland and sets $DISPLAY at the same time. Find $DISPLAY by finding
	// the Xwayland process and examining its args.
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		level.Debug(r.logger).Log(
			"msg", "could not query processes to find Xwayland process",
			"err", err,
		)
		return defaultDisplay
	}

	for _, p := range processes {
		cmdline, err := p.CmdlineWithContext(ctx)
		if err != nil {
			level.Debug(r.logger).Log(
				"msg", "could not get cmdline slice for process",
				"err", err,
			)
			continue
		}

		if !strings.Contains(cmdline, "Xwayland") {
			continue
		}

		// We have an Xwayland process -- check to make sure it's for our running user
		uids, err := p.UidsWithContext(ctx)
		if err != nil {
			level.Debug(r.logger).Log(
				"msg", "could not get uids for process",
				"err", err,
			)
			continue
		}
		uidMatch := false
		for _, procUid := range uids {
			if procUid == uid {
				uidMatch = true
				break
			}
		}

		if uidMatch {
			// We have a match! Grab the display value. The xwayland process looks like:
			// /usr/bin/Xwayland :0 -rootless -noreset -accessx -core -auth /run/user/1000/.mutter-Xwaylandauth.ROP401 -listen 4 -listen 5 -displayfd 6 -initfd 7
			cmdlineArgs := strings.Split(cmdline, " ")
			if len(cmdlineArgs) < 2 {
				// Process is somehow malformed or not what we're looking for -- continue so we can evaluate the following process
				continue
			}

			return cmdlineArgs[1]
		}
	}

	return defaultDisplay
}
