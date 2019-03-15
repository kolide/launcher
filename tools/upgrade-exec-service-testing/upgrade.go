package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

func triggerUpgrade(ctx context.Context, cancel func(), logger log.Logger) error {
	level.Info(logger).Log(
		"msg", "Starting Upgrade",
		"origpid", ProcessNotes.Pid,
	)

	// Should this get a random append?
	stagedFile := fmt.Sprintf("%s-staged", ProcessNotes.Path)

	// To emulate a new version, just copy the current binary to the staged location
	level.Debug(logger).Log("msg", "fs.CopyFile")
	if err := fs.CopyFile(ProcessNotes.Path, stagedFile); err != nil {
		return (errors.Wrap(err, "fs.CopyFile"))
	}

	oldFile := fmt.Sprintf("%s-old", ProcessNotes.Path)
	level.Debug(logger).Log("msg", "os.Rename cur to old")
	if err := os.Rename(ProcessNotes.Path, oldFile); err != nil {
		return errors.Wrap(err, "os.Rename cur top old")
	}

	level.Debug(logger).Log("msg", "os.Rename stage to cur")
	if err := os.Rename(stagedFile, ProcessNotes.Path); err != nil {
		return errors.Wrap(err, "os.Rename staged to cur")
	}

	level.Debug(logger).Log("msg", "os.Chmod")
	if err := os.Chmod(ProcessNotes.Path, 0755); err != nil {
		return errors.Wrap(err, "os.Chmod")
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
		return errors.Wrap(err, "syscall.Exec")
	}

	// Getting here, means the exec call returned
	return nil
}
