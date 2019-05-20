// +build darwin
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"syscall"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

func (uo *UninstallOptions) Uninstall(ctx context.Context) error {
	launchdFile := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", uo.identifier)
	desktopAppPrefix := "/Applications/Kolide.app"

	logger := ctxlog.FromContext(ctx)

	level.Debug(logger).Log(
		"msg", "starting uninstall",
		"platform", runtime.GOOS,
		"identifier", uo.identifier,
	)

	// Check root
	user, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "getting current user")
	}
	if user.Username != "root" {
		fmt.Println("\n\nYou need to be root. Consider sudo\n\n")
		return errors.New("not root")
	}

	// Prompt user
	if err := uo.promptUser(
		fmt.Sprintf("This will completely remove Kolide Launcher for %s", uo.identifierHumanName),
	); err != nil {
		return errors.Wrap(err, "prompted")
	}

	// Unload and  remove launchd
	if _, err := os.Stat(launchdFile); err == nil {
		if _, _, err := uo.execOut(ctx, "launchctl", "unload", launchdFile); err != nil {
			return errors.Wrap(err, "launchctl unload")
		}

		if err := uo.removePath(ctx, launchdFile); err != nil {
			return errors.Wrapf(err, "Removing %s", launchdFile)
		}
	} else if os.IsNotExist(err) {
		level.Debug(logger).Log("msg", "Can't unload launchd, already already gone", "path", launchdFile)
	} else {
		return errors.Wrapf(err, "Unable to tell if %s exists", launchdFile)
	}

	for _, dirf := range []string{"/usr/local/%s", "/etc/%s", "/var/%s"} {
		dir := fmt.Sprintf(dirf, uo.identifier)
		if err := uo.removePath(ctx, dir); err != nil {
			return errors.Wrapf(err, "Removing %s", dir)
		}
	}

	// Stop the desktop app, if running.
	// Blindly use pkill
	if err := uo.stopDesktop(ctx, desktopAppPrefix); err != nil {
		return errors.Wrap(err, "stopping desktop app")
	}

	// Remove the desktop app
	if err := uo.removePath(ctx, desktopAppPrefix); err != nil {
		return errors.Wrap(err, "Removing")
	}

	return nil
}

func (uo *UninstallOptions) stopDesktop(ctx context.Context, prefix string) error {
	logger := ctxlog.FromContext(ctx)

	_, _, err := uo.execOut(ctx, "pkill", "-f", prefix)
	if err != nil {
		if exitError, ok := errors.Cause(err).(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode := ws.ExitStatus()
			if exitCode == 1 {
				level.Debug(logger).Log("msg", "Desktop app already stopped")
				return nil
			}
			return errors.Wrap(err, "unknown exit code from pkill")
		}
		return errors.Wrap(err, "unknown error from pkill")
	}
	return nil
}
