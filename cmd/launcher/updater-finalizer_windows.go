// +build windows

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

// updateFinalizer finalizes a launcher update. As windows does not
// support an exec, we exit so the service manager will restart
// us. Exit(0) might be more correct, but that's harder to plumb
// through this stack. So, return an error here to trigger an exit
// higher in the stack.
func updateFinalizer(logger log.Logger, shutdownOsquery func() error) func() error {
	return func() error {
		if err := shutdownOsquery(); err != nil {
			level.Info(logger).Log("msg", "calling shutdownOsquery", "method", "updateFinalizer", "err", err)
			level.Debug(logger).Log("msg", "calling shutdownOsquery", "method", "updateFinalizer", "err", err, "stack", fmt.Sprintf("%+v", err))
		}

		// Use the FindNewest mechanism to delete old
		// updates. We do this here, as windows will pick up
		// the update in main, which does not delete.  Note
		// that this will likely produce non-fatal errors when
		// it tries to delete the running one.
		_ = autoupdate.FindNewest(
			ctxlog.NewContext(context.TODO(), logger),
			os.Args[0],
			autoupdate.DeleteOldUpdates(),
		)

		level.Info(logger).Log("msg", "Exiting launcher to allow a service manager to start the new one")
		return autoupdate.NewLauncherRestartNeededErr("Exiting launcher to allow a service manager restart")
	}
}
