//go:build darwin
// +build darwin

package runner

import (
	"os/exec"
)

// For notifications to work, we must run in the user context with launchctl asuser.
func runAsUser(uid string, cmd *exec.Cmd) error {

	// the remaining code in this function is not covered by unit test since it requires root privileges
	// We may be able to run passwordless sudo in GitHub actions, could possibly exec the tests as sudo.
	// But we may not have a console user?

	cmd.Path = "/bin/launchctl"
	updatedCmdArgs := append([]string{"/bin/launchctl", "asuser", uid}, cmd.Args...)
	cmd.Args = updatedCmdArgs

	return cmd.Start()
}
