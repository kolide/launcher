//go:build linux
// +build linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	defaultDisplay        = ":0"
	defaultWaylandDisplay = "wayland-0"
)

// Display takes the format host:displaynumber.screen
var displayRegex = regexp.MustCompile(`^[a-z]*:\d+.?\d*$`)

func (r *DesktopUsersProcessesRunner) runAsUser(ctx context.Context, uid string, cmd *exec.Cmd) error {
	ctx, span := traces.StartSpan(ctx, "uid", uid)
	defer span.End()

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil || runningUser == nil {
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
	envVars := r.userEnvVars(ctx, uid, runningUser.Username)
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd.Start()
}

func (r *DesktopUsersProcessesRunner) userEnvVars(ctx context.Context, uid string, username string) map[string]string {
	envVars := make(map[string]string)

	uidInt, err := strconv.ParseInt(uid, 10, 32)
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not convert uid to int32",
			"err", err,
		)
		return envVars
	}

	// Get the user's session so we can get their display (needed for opening notification action URLs in browser)
	cmd, err := allowedcmd.Loginctl(ctx, "show-user", uid, "--value", "--property=Sessions")
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not create loginctl command",
			"uid", uid,
			"err", err,
		)
		return envVars
	}
	sessionOutput, err := cmd.Output()
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not get user session",
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
		cmd, err := allowedcmd.Loginctl(ctx, "show-session", session, "--value", "--property=Type")
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not create loginctl command to get session type",
				"uid", uid,
				"err", err,
			)
			continue
		}
		typeOutput, err := cmd.Output()
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not get session type",
				"uid", uid,
				"err", err,
			)
			continue
		}

		sessionType := strings.Trim(string(typeOutput), "\n")
		if sessionType == "x11" {
			envVars["DISPLAY"] = r.displayFromX11(ctx, session, int32(uidInt))
			break
		} else if sessionType == "wayland" {
			envVars["DISPLAY"] = r.displayFromDisplayServerProcess(ctx, int32(uidInt))
			envVars["WAYLAND_DISPLAY"] = r.getWaylandDisplay(ctx, uid)

			break
		}
	}

	// For opening links with xdg-open, we need XDG_DATA_DIRS so that xdg-open can find the mimetype configuration
	// files to figure out what application to launch.

	// We take the default value according to https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html,
	// but also include the snapd directory due to an issue on Ubuntu 22.04 where the default
	// /usr/share/applications/mimeinfo.cache does not contain any applications installed via snap.
	envVars["XDG_DATA_DIRS"] = "/usr/local/share/:/usr/share/:/var/lib/snapd/desktop"
	envVars["XDG_RUNTIME_DIR"] = getXdgRuntimeDir(uid)

	// We need xauthority set in order to launch the browser on Ubuntu 23.04
	if xauthorityLocation := r.getXauthority(ctx, uid, username); xauthorityLocation != "" {
		envVars["XAUTHORITY"] = xauthorityLocation
	}

	return envVars
}

func (r *DesktopUsersProcessesRunner) displayFromX11(ctx context.Context, session string, uid int32) string {
	// We can read $DISPLAY from the session properties
	cmd, err := allowedcmd.Loginctl(ctx, "show-session", session, "--value", "--property=Display")
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not create command to get Display from user session",
			"err", err,
		)
		return r.displayFromDisplayServerProcess(ctx, uid)
	}
	xDisplayOutput, err := cmd.Output()
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not get Display from user session",
			"err", err,
		)
		return r.displayFromDisplayServerProcess(ctx, uid)
	}

	display := strings.Trim(string(xDisplayOutput), "\n")
	if display == "" {
		return r.displayFromDisplayServerProcess(ctx, uid)
	}

	return display
}

func (r *DesktopUsersProcessesRunner) displayFromDisplayServerProcess(ctx context.Context, uid int32) string {
	// Sometimes we can't get DISPLAY from loginctl show-session output.
	// We can look for it instead by looking for the display server process,
	// and examining its args and open socket connections.
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not query processes to find display server process",
			"err", err,
		)
		return defaultDisplay
	}

	for _, p := range processes {
		cmdline, err := p.CmdlineWithContext(ctx)
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not get cmdline slice for process",
				"err", err,
			)
			continue
		}

		if !strings.Contains(cmdline, "Xorg") && !strings.Contains(cmdline, "Xvfb") && !strings.Contains(cmdline, "Xwayland") {
			continue
		}

		// We have a display server process -- check to make sure it's for our running user
		uids, err := p.UidsWithContext(ctx)
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not get uids for process",
				"err", err,
			)
			continue
		}
		uidMatch := false
		for _, procUid := range uids {
			if procUid == uint32(uid) {
				uidMatch = true
				break
			}
		}
		if !uidMatch {
			continue
		}

		// We have a match! Grab the DISPLAY value from the display server process's connection to the display socket.
		display, err := r.getDisplayFromDisplayServerConnections(ctx, p.Pid)
		if err == nil {
			return display
		}
		r.slogger.Log(ctx, slog.LevelWarn,
			"could not extract display from open connections for process",
			"pid", p.Pid,
			"cmdline", cmdline,
			"err", err,
		)

		// We weren't able to get the DISPLAY value from the open connections. Try to parse it from the command-line args instead.
		// The xwayland process looks like:
		// /usr/bin/Xwayland :0 -rootless -noreset -accessx -core -auth /run/user/1000/.mutter-Xwaylandauth.ROP401 -listen 4 -listen 5 -displayfd 6 -initfd 7
		// The Xorg process may look like:
		// /usr/lib/xorg/Xorg :20 -auth /home/<user>/.Xauthority -nolisten tcp -noreset -logfile /dev/null -verbose 3 -config /tmp/chrome_remote_desktop_j5rldjlk.conf
		// The Xvfb process looks like:
		// Xvfb :20 -auth /home/<user>/.Xauthority -nolisten tcp -noreset -screen 0 3840x2560x24
		cmdlineArgs := strings.Split(cmdline, " ")
		if len(cmdlineArgs) < 2 {
			// Process is somehow malformed or not what we're looking for -- continue so we can evaluate the following process
			continue
		}

		if displayRegex.MatchString(cmdlineArgs[1]) {
			return cmdlineArgs[1]
		}
	}

	return defaultDisplay
}

const displaySocketPrefix = "/tmp/.X11-unix/X"

// getDisplayFromDisplayServerConnections looks at the open connections from the given PID,
// looking for the display socket to map to the correct display.
func (r *DesktopUsersProcessesRunner) getDisplayFromDisplayServerConnections(ctx context.Context, pid int32) (string, error) {
	openConns, err := net.ConnectionsPidWithContext(ctx, "unix", pid)
	if err != nil {
		return "", fmt.Errorf("getting open connections for display server process: %w", err)
	}

	for _, openConn := range openConns {
		if !strings.HasPrefix(openConn.Laddr.IP, displaySocketPrefix) {
			continue
		}

		// We have the socket -- extract the display number
		potentialDisplayNum := strings.TrimPrefix(openConn.Laddr.IP, displaySocketPrefix)
		displayNum, err := strconv.Atoi(potentialDisplayNum)
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not parse display number from display socket",
				"socket", openConn.String(),
				"err", err,
			)
			continue
		}

		return fmt.Sprintf(":%d", displayNum), nil
	}

	return "", fmt.Errorf("socket not found for pid %d", pid)
}

// getWaylandDisplay returns the appropriate value to set as WAYLAND_DISPLAY
func (r *DesktopUsersProcessesRunner) getWaylandDisplay(ctx context.Context, uid string) string {
	// Find the wayland display socket
	waylandDisplaySocketPattern := filepath.Join(getXdgRuntimeDir(uid), "wayland-*")
	matches, err := filepath.Glob(waylandDisplaySocketPattern)
	if err != nil || len(matches) == 0 {
		r.slogger.Log(ctx, slog.LevelDebug,
			"could not get wayland display from xdg runtime dir",
			"err", err,
		)
		return defaultWaylandDisplay
	}

	// We may also match a lock file wayland-0.lock, so iterate through matches and only return the one that's a socket
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			r.slogger.Log(ctx, slog.LevelDebug,
				"could not stat potential wayland display socket",
				"file_path", match,
				"err", err,
			)
			continue
		}

		if info.Mode().Type() == fs.ModeSocket {
			return filepath.Base(match)
		}
	}

	return defaultWaylandDisplay
}

// getXauthority checks known locations for the xauthority file
func (r *DesktopUsersProcessesRunner) getXauthority(ctx context.Context, uid string, username string) string {
	xdgRuntimeDir := getXdgRuntimeDir(uid)

	// Glob for Wayland matches first
	waylandXAuthorityLocationPattern := filepath.Join(xdgRuntimeDir, ".mutter-Xwaylandauth.*")
	if matches, err := filepath.Glob(waylandXAuthorityLocationPattern); err == nil && len(matches) > 0 {
		return matches[0]
	}

	// Next, check default X11 location
	x11XauthorityLocation := filepath.Join(xdgRuntimeDir, "gdm", "Xauthority")
	if _, err := os.Stat(x11XauthorityLocation); err == nil {
		return x11XauthorityLocation
	}

	// Default location is $HOME/.Xauthority -- try that before giving up
	homeLocation := filepath.Join("/home", username, ".Xauthority")
	if _, err := os.Stat(homeLocation); err == nil {
		return homeLocation
	}

	r.slogger.Log(ctx, slog.LevelDebug,
		"could not find xauthority in any known location",
		"wayland_location", waylandXAuthorityLocationPattern,
		"x11_location", x11XauthorityLocation,
		"default_location", homeLocation,
	)

	return ""
}

func getXdgRuntimeDir(uid string) string {
	return fmt.Sprintf("/run/user/%s", uid)
}

func osversion() (string, error) {
	return "", errors.New("not implemented")
}

// logIndicatesSystrayNeedsRestart is Windows-only functionality
func logIndicatesSystrayNeedsRestart(_ string) bool {
	return false
}
