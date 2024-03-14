//go:build !windows
// +build !windows

package tablehelpers

import (
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

func RunCmdAsUser(cmd *exec.Cmd, uid string) ([]byte, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
		return nil, fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	if currentUser.Uid != "0" && currentUser.Uid != runningUser.Uid {
		return nil, fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
	}

	runningUserUid, err := strconv.ParseUint(runningUser.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("converting uid %s to int: %w", runningUser.Uid, err)
	}

	runningUserGid, err := strconv.ParseUint(runningUser.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("converting gid %s to int: %w", runningUser.Gid, err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(runningUserUid),
			Gid: uint32(runningUserGid),
		},
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("assigning command StdoutPipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	data, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("ReadAll command stdout: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("deallocate command: %w", err)
	}

	return data, nil
}
