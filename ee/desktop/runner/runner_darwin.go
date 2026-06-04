//go:build darwin

package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/pkg/backoff"
	"golang.org/x/sys/unix"
)

// For notifications to work, we must run in the user context with launchctl asuser.
func (r *DesktopUsersProcessesRunner) runAsUser(ctx context.Context, uid string, cmd *exec.Cmd) error {
	_, span := observability.StartSpan(ctx, "uid", uid)
	defer span.End()

	// Ensure that we handle a non-root current user appropriately
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil || runningUser == nil {
		return fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	// Update command so that we're prepending `launchctl asuser $UID sudo --preserve-env -u $runningUser` to the launcher desktop command.
	// We need to run with `launchctl asuser` in order to get the user context, which is required to be able to send notifications.
	// We need `sudo -u $runningUser` to set the UID on the command correctly -- necessary for, among other things, correctly observing
	// light vs dark mode.
	// We need --preserve-env for sudo in order to avoid clearing SOCKET_PATH, AUTHTOKEN, etc that are necessary for the desktop
	// process to run.
	cmd.Path = "/bin/launchctl"
	updatedCmdArgs := append([]string{"/bin/launchctl", "asuser", uid, "sudo", "--preserve-env", "-u", runningUser.Username}, cmd.Args...)
	cmd.Args = updatedCmdArgs

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

	return cmd.Start()
}

func osversion() (string, error) {
	return unix.Sysctl("kern.osrelease")
}

// logIndicatesSystrayNeedsRestart is Windows-only functionality
func logIndicatesSystrayNeedsRestart(_ string) bool {
	return false
}

// waitForReadyToSpawnDesktopState repeatedly checks to see if the ControlCenter is running,
// since we need it for our menu bar icon to appear.
func (r *DesktopUsersProcessesRunner) waitForReadyToSpawnDesktopState(ctx context.Context, uid string) {
	if err := backoff.WaitFor(func() error {
		controlCenterService := fmt.Sprintf("gui/%s/com.apple.controlcenter", uid)
		cmd, err := allowedcmd.Launchctl.Cmd(ctx, "print", controlCenterService)
		if err != nil {
			return fmt.Errorf("creating `launchctl print %s` command: %w", controlCenterService, err)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running `launchctl print %s`: %w", controlCenterService, err)
		}
		outStr := string(out)
		// The output for launchctl print is not guaranteed to remain the same,
		// so we have a couple different checks here to see if we have a running Control Center.
		// First, we check for "state = running" in the output. If it's there, then
		// we assume the service is running.
		if strings.Contains(outStr, "state = running") {
			return nil
		}
		// Next, we look for our expected error string, "Could not find service".
		// Usually when we see this case, launchctl print exits with a non-zero error code,
		// returning an error on `CombinedOutput` above. But just in case that changes,
		// we check for the error output string here as well.
		if strings.Contains(outStr, "Could not find service") {
			return fmt.Errorf("%s not yet running", controlCenterService)
		}
		// If we make it here, we assume the service isn't running yet. We separate
		// this case out from the above for improving output parsing in the future.
		return fmt.Errorf("unexpected launchctl output, assuming service is not running: %s", outStr)
	}, 30*time.Second, 1*time.Second); err != nil {
		r.slogger.Log(ctx, slog.LevelWarn,
			"ControlCenter process not found before timeout",
			"err", err,
		)
	}
}
