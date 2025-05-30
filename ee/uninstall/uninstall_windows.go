package uninstall

import (
	"context"
	"fmt"
	"time"

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

	// Recovery actions happen even when we have StartManual set, so we need to clear
	// those as well and replace with a NoAction action.
	if err := launcherSvc.SetRecoveryActions([]mgr.RecoveryAction{
		{
			Type:  mgr.NoAction,
			Delay: 5 * time.Second,
		},
	}, 24*60*60); err != nil {
		return fmt.Errorf("resetting recovery actions: %w", err)
	}

	// attempt to remove watchdog service in case it is installed to prevent startups later on
	if err := watchdog.RemoveWatchdogTask(k.Identifier()); err != nil {
		return fmt.Errorf("removing watchdog task, error may be expected if not installed: %w", err)
	}

	return nil
}
