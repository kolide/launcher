package uninstall

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/watchdog"
	"github.com/kolide/launcher/pkg/launcher"
	"golang.org/x/sys/windows/svc/mgr"
)

func disableAutoStart(ctx context.Context, k types.Knapsack) error {
	svcMgr, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to windows service manager: %w", err)
	}
	defer svcMgr.Disconnect()

	serviceName := launcher.ServiceName(k.Identifier())
	launcherSvc, err := svcMgr.OpenService(serviceName)
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

	// attempt to remove watchdog service in case it is installed to prevent startups later on
	if err := watchdog.RemoveWatchdogTask(k.Identifier()); err != nil {
		return fmt.Errorf("removing watchdog task, error may be expected if not installed: %w", err)
	}

	return nil
}
