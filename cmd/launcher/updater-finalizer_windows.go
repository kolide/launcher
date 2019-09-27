// +build windows

package main

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
)

// updateFinalizer finalizes a launcher update. As windows does not
// support an exec, we exit so the service manager will restart
// us. Exit(0) might be more correct, but that's harder to plumb
// through this stack. So, return an error here to trigger an exit
// higher in the stack.
func updateFinalizer(logger log.Logger, shutdownOsquery func() error) func() error {
	return func() error {
		if err := shutdownOsquery(); err != nil {
			level.Info(logger).Log(
				"msg", "calling shutdownOsquery",
				"method", "updateFinalizer",
				"err", err,
				"stack", fmt.Sprintf("%+v", err),
			)
		}
		level.Info(logger).Log("msg", "Exiting launcher to allow a service manager to start the new one")
		return autoupdate.NewLauncherRestartNeededErr("Exiting launcher to allow a service manager restart")
	}
}
