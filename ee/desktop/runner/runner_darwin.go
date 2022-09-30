//go:build darwin
// +build darwin

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/go-kit/kit/log/level"
)

func (r *DesktopUsersProcessesRunner) consoleUsers() ([]string, error) {
	consoleOwnerUid, err := consoleOwnerUid()
	if err != nil {
		return nil, fmt.Errorf("getting console owner uid: %w", err)
	}

	// there seems to be a brief moment during start up where root or system (non-human)
	// users own the console, if we spin up the process for them it will add an
	// unnecessary process. On macOS human users start at 501
	if consoleOwnerUid < 501 {
		level.Debug(r.logger).Log(
			"msg", "skipping desktop for root or system user",
			"uid", consoleOwnerUid,
		)

		return []string{}, nil
	}

	return []string{fmt.Sprint(consoleOwnerUid)}, nil
}

func consoleOwnerUid() (uint32, error) {
	const consolePath = "/dev/console"
	consoleInfo, err := os.Stat(consolePath)
	if err != nil {
		return uint32(0), fmt.Errorf("stat %s: %w", consolePath, err)
	}

	return consoleInfo.Sys().(*syscall.Stat_t).Uid, nil
}

func cmdAsUser(uid string, path string, args ...string) (*exec.Cmd, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
		return nil, fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	cmd := exec.Command(path, args...)

	// current user not root
	if currentUser.Uid != "0" {
		// if the user is running for itself, just run without setting credentials
		if currentUser.Uid == uid {
			return cmd, nil
		}

		// if the user is running for another user, we have an error because we can't set credentials
		return nil, fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
	}

	// the remaining code in this function is not covered by unit test since it requires root privileges
	// We may be able to run passwordless sudo in GitHub actions, could possibly exec the tests as sudo.
	// But we may not have a console user?

	uidInt, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("converting uid to int: %w", err)
	}

	gid, err := strconv.ParseUint(runningUser.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("converting gid to int: %w", err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uidInt),
			Gid: uint32(gid),
		},
	}

	return cmd, nil
}
