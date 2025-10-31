//go:build windows
// +build windows

package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/systray"
)

func (r *DesktopUsersProcessesRunner) runAsUser(ctx context.Context, consoleUser *consoleuser.ConsoleUser, cmd *exec.Cmd) error {
	_, span := observability.StartSpan(ctx, "uid", consoleUser.Uid)
	defer span.End()

	// get the access token of the user that owns the explorer process
	// and use it to spawn a new process as that user
	accessToken, err := processAccessToken(consoleUser.UserProcessPid)
	if err != nil {
		return fmt.Errorf("getting explorer process token: %w", err)
	}
	defer accessToken.Close()

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: accessToken,
	}

	return cmd.Start()
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

func osversion() (string, error) {
	return "", errors.New("not implemented")
}

// logIndicatesSystrayNeedsRestart checks to see if the log line contains
// "tray not ready yet", which indicates that the systray had an irrecoverable
// error during initialization and requires restart. Sometimes the tray may
// also fail to initialize with "Unspecified error", so we check for the generic
// initialization failed message as well.
func logIndicatesSystrayNeedsRestart(logLine string) bool {
	return strings.Contains(logLine, systray.ErrTrayNotReadyYet.Error()) ||
		strings.Contains(logLine, "systray error: unable to init instance")
}
