// +build linux
package main

import (
	"context"
	"fmt"
	"os/user"
	"runtime"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

func (uo *UninstallOptions) Uninstall(ctx context.Context) error {

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

	return nil
}
