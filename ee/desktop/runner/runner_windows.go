//go:build windows
// +build windows

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"
)

// runConsoleUserDesktop iterates over all the current explorer processes and
// runs the desktop process for the owner if none currently exists
func (r *DesktopUsersProcessesRunner) consoleUsers() ([]string, error) {
	explorerProcs, err := explorerProcesses()
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	var consoleUsers []string

	for _, explorerProc := range explorerProcs {
		uid, err := processOwnerUid(explorerProc)
		if err != nil {
			return nil, fmt.Errorf("getting process owner uid: %w", err)
		}
		consoleUsers = append(consoleUsers, uid)
	}

	return consoleUsers, nil
}

func cmdAsUser(uid string, path string, args ...string) (*exec.Cmd, error) {
	explorerProc, err := userExplorerProcess(uid)
	if err != nil {
		return nil, fmt.Errorf("getting user explorer process: %w", err)
	}

	// get the access token of the user that owns the explorer process
	// and use it to spawn a new process as that user
	accessToken, err := processAccessToken(explorerProc.Pid)

	if err != nil {
		return nil, fmt.Errorf("getting explorer process token: %w", err)
	}

	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: accessToken,
	}

	return cmd, nil
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

// explorerProcesses returns a list of explorer processes whose
// filepath base is "explorer.exe".
func explorerProcesses() ([]*process.Process, error) {
	var explorerProcs []*process.Process

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting processes: %w", err)
	}

	for _, proc := range procs {
		exe, err := proc.ExeWithContext(ctx)
		if err != nil {
			continue
		}

		if filepath.Base(exe) == "explorer.exe" {
			explorerProcs = append(explorerProcs, proc)
		}
	}

	return explorerProcs, nil
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
