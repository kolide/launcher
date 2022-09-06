//go:build windows
// +build windows

package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/go-kit/kit/log/level"
	"github.com/shirou/gopsutil/process"
)

// runDesktopNative iterates over all the current explorer processes and
// runs the desktop process for the owner if none currently exists
func (r *DesktopUsersProcessesRunner) runConsoleUserDesktop() error {
	explorerProcs, err := explorerProcesses()
	if err != nil {
		return fmt.Errorf("getting explorer processes: %w", err)
	}

	for _, explorerProc := range explorerProcs {
		uid, err := processOwnerUid(explorerProc)
		if err != nil {
			return fmt.Errorf("getting process owner uid: %w", err)
		}

		// already have a process, move on
		if r.userHasDesktopProcess(uid) {
			continue
		}

		executablePath, err := r.determineExecutablePath()
		if err != nil {
			return fmt.Errorf("determining executable path: %w", err)
		}

		// get the access token of the user that owns the explorer process
		// and use it to spawn a new process as that user
		accessToken, err := processAccessToken(explorerProc.Pid)
		if err != nil {
			return fmt.Errorf("getting explorer process token: %w", err)
		}
		defer accessToken.Close()

		proc, err := runWithAccessToken(accessToken, executablePath, "desktop")
		if err != nil {
			return fmt.Errorf("running desktop: %w", err)
		}

		if err := r.addDesktopProcess(uid, proc); err != nil {
			return fmt.Errorf("adding desktop process: %w", err)
		}

		level.Debug(r.logger).Log(
			"msg", "desktop started",
			"uid", uid,
			"pid", proc.Pid,
		)

		r.waitOnProcessAsync(uid, proc)
	}

	return nil
}

// explorerProcesses returns a list of explorer processes whose
// filepath base is "explorer.exe".
func explorerProcesses() ([]*process.Process, error) {
	var explorerProcs []*process.Process

	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("getting processes: %w", err)
	}

	for _, proc := range procs {
		exe, err := proc.Exe()
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

// runWithAccessToken runs the given executable with the given arguments using the given access token
func runWithAccessToken(accessToken syscall.Token, path string, args ...string) (*os.Process, error) {
	cmd := exec.Command(path, args...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: accessToken,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command with access token: %w", err)
	}

	return cmd.Process, nil
}
