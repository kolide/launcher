//go:build windows
// +build windows

package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/process"
)

func setpgid() *syscall.SysProcAttr {
	// TODO: on unix we set the process group id and then
	// terminate that process group.
	return &syscall.SysProcAttr{}
}

func killProcessGroup(origCmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// some discussion here https://github.com/golang/dep/pull/857
	cmd, err := allowedcmd.Taskkill(ctx, "/F", "/T", "/PID", fmt.Sprint(origCmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("running taskkill: output: %s, err: %w", string(out), err)
		}

		if ctx.Err() != nil {
			return fmt.Errorf("running taskkill: context err: %v, err: %w", ctx.Err(), err)
		}

		return fmt.Errorf("running taskkill: err: %w", err)
	}

	return nil
}

func SocketPath(rootDir string, id string) string {
	// On windows, local names pipes paths are all rooted in \\.\pipe\
	// their names are limited to 256 characters, and can include any
	// character other than backslash. They are case insensitive.
	//
	// They have some set of security parameters, which can be set at
	// create time. They are automatically removed when the last handle
	// to pipe is closed.
	//
	// Our usage of the pipe is for shared communication between
	// launcher and osquery. We would like to be able to run multiple
	// launchers.
	//
	// We could use something based on the launcher root, but given the
	// context this runs in a ulid seems simpler.
	return fmt.Sprintf(`\\.\pipe\kolide-osquery-%s`, id)
}

func platformArgs() []string {
	return []string{
		"--allow_unsafe",
	}
}

func isExitOk(err error) bool {
	if exiterr, ok := errors.Cause(err).(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			// https://msdn.microsoft.com/en-us/library/cc704588.aspx
			// STATUS_CONTROL_C_EXIT
			return status.ExitStatus() == 3221225786
		}
	}
	return false
}

func getProcessesHoldingFile(ctx context.Context, pathToFile string) ([]*process.Process, error) {
	allProcesses, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting process list: %w", err)
	}
	if len(allProcesses) == 0 {
		return nil, errors.New("could not get any processes")
	}

	processes := make([]*process.Process, 0)
	for _, p := range allProcesses {
		openFiles, err := p.OpenFilesWithContext(ctx)
		if err != nil {
			continue
		}

		// Check the process's open files to see if this process is the one using the lockfile
		for _, f := range openFiles {
			// We check for strings.Contains rather than equals because the open file's path contains
			// a `\\?\` prefix.
			if !strings.Contains(f.Path, pathToFile) {
				continue
			}

			processes = append(processes, p)
			break
		}
	}

	if len(processes) == 0 {
		return nil, fmt.Errorf("no processes found using file %s", pathToFile)
	}

	return processes, nil
}
