//go:build windows
// +build windows

package main

import (
	"fmt"

	"github.com/kardianos/osext"

	"golang.org/x/sys/windows/svc/mgr"
)

func runInstallService(args []string) error {
	exepath, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("osext.Executable: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	cfg := mgr.Config{DisplayName: serviceDesc, StartType: mgr.StartAutomatic}

	//ra := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 5 * time.Second}

	s, err = m.CreateService(serviceName, exepath, cfg, "svc")
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer s.Close()

	//if err := s.SetRecoveryActions([]mgr.RecoveryAction{ra}, 3); err != nil {
	//return errors.Wrap(err, "SetRecoveryActions")
	//}

	return nil
}

func runRemoveService(args []string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		s.Close()
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	err = s.Delete()
	return err

}
