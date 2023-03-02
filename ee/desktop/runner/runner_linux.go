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
	"github.com/shirou/gopsutil/process"
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
			// For opening links with x-www-browser, we only need DISPLAY.
			runningUserUid, err := strconv.ParseInt(uid, 10, 32)
			if err != nil {
				level.Debug(r.logger).Log(
					"msg", "could not convert uid to int32",
					"err", err,
				)
				break CheckSessions
			}
			envVars["DISPLAY"] = r.displayFromXwayland(int32(runningUserUid))

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

func (r *DesktopUsersProcessesRunner) displayFromXwayland(uid int32) string {
	//For wayland, DISPLAY is not included in loginctl show-session output -- in GNOME,
	// Mutter spawns Xwayland and sets $DISPLAY at the same time. Find DISPLAY by finding
	// the Xwayland process and examining its args.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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

		for _, procUid := range uids {
			if procUid != uid {
				continue
			}

			// We have a match! Grab the display value. The xwayland process looks like:
			// /usr/bin/Xwayland :0 -rootless -noreset -accessx -core -auth /run/user/1000/.mutter-Xwaylandauth.ROP401 -listen 4 -listen 5 -displayfd 6 -initfd 7
			cmdlineArgs := strings.Split(cmdline, " ")
			if len(cmdlineArgs) < 2 {
				// Process is somehow malformed or not what we're looking for -- stop evaluating it
				break
			}

			return cmdlineArgs[1]
		}
	}

	return defaultDisplay
}
