package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
)

func triggerUpgrade(ctx context.Context, cancel func(), logger log.Logger) error {
	level.Info(logger).Log(
		"msg", "Starting Upgrade",
		"origpid", ProcessNotes.Pid,
	)

	// Should this get a random append?
	stagedFile := fmt.Sprintf("%s-staged", ProcessNotes.Path)

	// To emulate a new version, just copy the current binary to the staged location
	level.Debug(logger).Log("msg", "fsutil.CopyFile")
	if err := fsutil.CopyFile(ProcessNotes.Path, stagedFile); err != nil {
		return (fmt.Errorf("fsutil.CopyFile: %w", err))
	}

	oldFile := fmt.Sprintf("%s-old", ProcessNotes.Path)
	level.Debug(logger).Log("msg", "os.Rename cur to old")
	if err := os.Rename(ProcessNotes.Path, oldFile); err != nil {
		return fmt.Errorf("os.Rename cur top old: %w", err)
	}

	level.Debug(logger).Log("msg", "os.Rename stage to cur")
	if err := os.Rename(stagedFile, ProcessNotes.Path); err != nil {
		return fmt.Errorf("os.Rename staged to cur: %w", err)
	}

	level.Debug(logger).Log("msg", "os.Chmod")
	if err := os.Chmod(ProcessNotes.Path, 0755); err != nil {
		return fmt.Errorf("os.Chmod: %w", err)
	}

	// Our normal process here is to exec the new binary. However, this
	// doesn't work on windows -- windows has no exec. So instead, we
	// exit, and let the service manager restart us.
	if runtime.GOOS == "windows" {
		level.Info(logger).Log("msg", "Exiting, so service manager can restart new version")
		return nil
	}

	// For non-windows machine, exec the new version
	level.Debug(logger).Log("msg", "syscall.Exec")
	if err := syscall.Exec(ProcessNotes.Path, os.Args, os.Environ()); err != nil {
		return fmt.Errorf("syscall.Exec: %w", err)
	}

	// Getting here, means the exec call returned
	return nil
}
