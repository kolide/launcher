//go:build !windows
// +build !windows

package tablehelpers

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// WithUid is a functional argument which modifies the input exec command to run as a specific user.
func WithUid(uid string) ExecOps {
	return func(cmd *exec.Cmd) error {
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("getting current user: %w", err)
		}

		runningUser, err := user.LookupId(uid)
		if err != nil {
			return fmt.Errorf("looking up user with uid %s: %w", uid, err)
		}

		// If the current user is the user to run as, then early return to avoid needing NoSetGroups.
		if currentUser.Uid == runningUser.Uid {
			return nil
		} else if currentUser.Uid != "0" {
			return fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
		}

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

		// Set PWD and HOME to the running user's home directory for supporting executes that use them as the prefix to create temp files.
		cmd.Env = append(cmd.Environ(), "PWD="+runningUser.HomeDir, "HOME="+runningUser.HomeDir)

		return nil
	}
}
