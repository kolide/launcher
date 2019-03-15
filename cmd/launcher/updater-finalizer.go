// +build !windows

package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		// replace launcher
		level.Info(logger).Log("msg", "Exec for updated launcher")
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}
}
