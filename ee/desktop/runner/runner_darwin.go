//go:build darwin
// +build darwin

package runner

import (
	"fmt"
	"os/exec"
	"os/user"
)

// For notifications to work, we must run in the user context with launchctl asuser.
func runAsUser(uid string, cmd *exec.Cmd) error {
	// Update command so that we're prepending `launchctl asuser $UID` to the launcher desktop command
	cmd.Path = "/bin/launchctl"
	updatedCmdArgs := append([]string{"/bin/launchctl", "asuser", uid}, cmd.Args...)
	cmd.Args = updatedCmdArgs

	// Ensure that we handle a non-root current user appropriately
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

	return cmd.Start()
}
