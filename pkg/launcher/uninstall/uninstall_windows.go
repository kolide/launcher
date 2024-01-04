package uninstall

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc/mgr"
)

func disableAutoStart(ctx context.Context) error {
	svcMgr, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to windows service manager: %w", err)
	}
	defer svcMgr.Disconnect()

	launcherSvc, err := svcMgr.OpenService("LauncherKolideK2Svc")
	if err != nil {
		return fmt.Errorf("opening launcher service: %w", err)
	}
	defer launcherSvc.Close()

	cfg, err := launcherSvc.Config()
	if err != nil {
		return fmt.Errorf("getting launcher service config: %w", err)
	}

	cfg.StartType = mgr.StartManual
	if err := launcherSvc.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("updating launcher service config: %w", err)
	}

	return nil
}
