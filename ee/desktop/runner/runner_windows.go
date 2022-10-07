//go:build windows
// +build windows

package runner

import (
	"fmt"
	"os/exec"
	"os/user"
	"syscall"

	"github.com/shirou/gopsutil/process"
)

func runAsUser(uid string, cmd *exec.Cmd) error {
	explorerProc, err := userExplorerProcess(uid)
	if err != nil {
		return fmt.Errorf("getting user explorer process: %w", err)
	}

	// get the access token of the user that owns the explorer process
	// and use it to spawn a new process as that user
	accessToken, err := processAccessToken(explorerProc.Pid)
	if err != nil {
		return fmt.Errorf("getting explorer process token: %w", err)
	}
	defer accessToken.Close()

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: accessToken,
	}

	return cmd.Start()
}

func userExplorerProcess(uid string) (*process.Process, error) {
	explorerProcs, err := explorerProcesses()
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	for _, proc := range explorerProcs {
		procOwnerUid, err := processOwnerUid(proc)
		if err != nil {
			return nil, fmt.Errorf("getting process owner uid: %w", err)
		}

		if uid == procOwnerUid {
			return proc, nil
		}
	}

	return nil, nil
}

func processOwnerUid(proc *process.Process) (string, error) {
	username, err := proc.Username()
	if err != nil {
		return "", fmt.Errorf("getting process username: %w", err)
	}

	user, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("looking up user: %w", err)
	}

	return user.Uid, nil
}

/*
Original code from https://blog.davidvassallo.me/2022/06/17/golang-in-windows-execute-command-as-another-user/
Thank you David Vassallo!
*/

// processAccessToken returns the access token of the process with the given pid
func processAccessToken(pid int32) (syscall.Token, error) {
	var token syscall.Token

	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return token, fmt.Errorf("opening process: %w", err)
	}
	defer syscall.CloseHandle(handle)

	// Find process token via win32
	if err := syscall.OpenProcessToken(handle, syscall.TOKEN_ALL_ACCESS, &token); err != nil {
		return token, fmt.Errorf("opening process token: %w", err)
	}

	return token, err
}
