//go:build linux
// +build linux

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/kit/log/level"
)

const defaultDisplay = ":0"

func (r *DesktopUsersProcessesRunner) runAsUser(uid string, cmd *exec.Cmd) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
		return fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	// current user not root
	if currentUser.Uid != "0" {
		// if the user is running for itself, just run without setting credentials
		if currentUser.Uid == runningUser.Uid {
			return cmd.Start()
		}

		// if the user is running for another user, we have an error because we can't set credentials
		return fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
	}

	// the remaining code in this function is not covered by unit test since it requires root privileges
	// We may be able to run passwordless sudo in GitHub actions, could possibly exec the tests as sudo.
	// But we may not have a console user?

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

	// Set any necessary environment variables on the command (like DISPLAY)
	envVars := r.userEnvVars(uid)
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd.Start()
}

func (r *DesktopUsersProcessesRunner) userEnvVars(uid string) map[string]string {
	envVars := make(map[string]string)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the user's session so we can get their display.
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

CheckSessions:
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
		switch sessionType {
		case "x11":
			// We need to set DISPLAY, which we can read from the session properties.
			xDisplayOutput, err := exec.CommandContext(ctx, "loginctl", "show-session", session, "--value", "--property=Display").Output()
			if err != nil {
				level.Debug(r.logger).Log(
					"msg", "could not get Display from user session",
					"uid", uid,
					"err", err,
				)
				continue
			}

			xDisplay := strings.Trim(string(xDisplayOutput), "\n")
			if xDisplay != "" {
				envVars["DISPLAY"] = xDisplay
				break CheckSessions
			}
		case "wayland":
			// For opening links with x-www-browser, we only need DISPLAY. For wayland, this is not included in
			// loginctl show-session output -- in GNOME, Mutter spawns Xwayland and sets $DISPLAY at the same time.
			envVars["DISPLAY"] = r.displayFromXwayland(uid)

			// For opening links with xdg-open, we need XDG_DATA_DIRS so that xdg-open can find the mimetype configuration
			// files to figure out what application to launch.

			// We take the default value according to https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html,
			// but also include the snapd directory due to an issue on Ubuntu 22.04 where the default
			// /usr/share/applications/mimeinfo.cache does not contain any applications installed via snap.
			envVars["XDG_DATA_DIRS"] = "/usr/local/share/:/usr/share/:/var/lib/snapd/desktop"

			break CheckSessions
		default:
			// Not a graphical session -- continue
			continue
		}
	}

	return envVars
}

func (r *DesktopUsersProcessesRunner) displayFromXwayland(uid string) string {
	// TODO
	// https://gitlab.gnome.org/GNOME/mutter/-/blob/main/src/wayland/meta-xwayland.c#L627
	// https://gitlab.gnome.org/GNOME/mutter/-/blob/main/src/wayland/meta-xwayland.c#L292
	// /tmp/.X%d-lock

	return defaultDisplay
}
