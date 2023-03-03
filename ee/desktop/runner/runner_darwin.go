//go:build darwin
// +build darwin

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
)

// For notifications to work, we must run in the user context with launchctl asuser.
func (r *DesktopUsersProcessesRunner) runAsUser(uid string, cmd *exec.Cmd, _ context.Context) error {
	// Ensure that we handle a non-root current user appropriately
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
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
