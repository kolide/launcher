// +build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// updateFinalizer finalizes a launcher update. It assume the new
// binary has been copied into place, and calls exec, so we start a
// new running launcher in our place.
func updateFinalizer(logger log.Logger, shutdownOsquery func() error) func() error {
	return func() error {
		if err := shutdownOsquery(); err != nil {
			level.Info(logger).Log(
				"method", "updateFinalizer",
				"err", err,
				"stack", fmt.Sprintf("%+v", err),
			)
		}
		// find the newest version of launcher on disk.
		// FindNewest uses context as a way to get a logger, so we need to create and pass one.
		// FIXME: add delete/cleanup here
		binaryPath := autoupdate.FindNewest(
			ctxlog.NewContext(context.TODO(), logger),
			os.Args[0],
		)

		// replace launcher
		level.Info(logger).Log(
			"msg", "Exec updated launcher",
			"newPath", binaryPath,
		)
		if err := syscall.Exec(binaryPath, os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "exec updated launcher")
		}
		return nil
	}
}
