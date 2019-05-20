// +build windows
package main

import (
	"context"
	"fmt"
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

	if err := uo.promptUser(
		fmt.Sprintf("This will completely remove Kolide Launcher for %s", uo.identifierHumanName),
	); err != nil {
		return errors.Wrap(err, "prompted")
	}

	return nil
}
