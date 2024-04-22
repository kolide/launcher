//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func stopService(service *mgr.Service) error {
	status, err := service.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stopping %s service: %w", service.Name, err)
	}

	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for %s service to stop", service.Name)
		}

		time.Sleep(500 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %w", err)
		}
	}

	return nil
}

func removeService(service *mgr.Service) error {
	if err := stopService(service); err != nil {
		return err
	}

	return service.Delete()
}

func removeLauncher(_ context.Context, _ string) error {
	// Uninstall is not implemented for Windows - users have to use add/remove programs themselves
	// return errors.New("Uninstall subcommand is not supported for Windows platforms.")

	sman, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service control manager: %w", err)
	}

	defer sman.Disconnect()

	restartService, err := sman.OpenService(launcherRestartServiceName)
	// would be better to check the individual error here but there will be one if the service does
	// not exist, in which case it's fine to just skip this anyway
	if err != nil {
		return err
	}

	defer restartService.Close()

	return removeService(restartService)
}
