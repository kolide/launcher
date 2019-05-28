// +build windows
package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
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

// removeCloudOsquery removes the pre-launcher cloud osquery
// configuration. This was a service, an MSI, and most importantly,
// some environmental variables.
func (uo *UninstallOptions) removeCloudOsquery(ctx context.Context) error {

	if uo.identifier == cloudIdentifier {
		backoff := backoff.New()
		stopWrapper := func() error {
			return uo.stopService("osqueryd")
		}
		if err := backoff.Run(stopWrapper); err != nil {
			return errors.Wrap(err, "stopping osqueryd")
		}
	}

	// Uninstall / remove program

	// delete data

	return nil
}

func (uo *UninstallOptions) stopService(name string) error {
	svcManager, err := mgr.Connect()
	if err != nil {
		return errors.Wrap(err, "connecting to service manager")
	}

	service, err := svcManager.OpenService(name)
	if err != nil {
		return errors.Wrapf(err, "finding %s service", name)
	}

	status, err := service.Query()
	if err != nil {
		return errors.Wrapf(err, "querying %s state", name)
	}

	// If we're stopped, or pending stopped, no need to no need to issue a stop block
	switch status.State {
	case svc.Stopped:
		return nil
	case svc.StopPending:
		time.Sleep(2 * time.Second)
		return errors.New("pending")
	}

	// Issue a stop, then pause for a second, and check the state. If
	// we're stopped, great. Otherwise, return an error and let backoff
	// handle it.
	newStatus, err := service.Control(svc.Stop)
	if err != nil {
		return errors.Wrapf(err, "sending stop to %s", name)
	}

	time.Sleep(2 * time.Second)

	switch newStatus.State {
	case svc.Stopped:
		return nil
	case svc.StopPending:
		time.Sleep(2 * time.Second)
		return errors.New("pending")
	}

	return errors.Errorf("Unable to stop %s", name)
}
