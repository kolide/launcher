//go:build windows
// +build windows

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"golang.org/x/sys/windows/registry"
)

const (
	regKeyMachineGuidParent string = `SOFTWARE\Microsoft\Cryptography`
	regValueMachineGuid     string = `MachineGuid`
)

// currentMachineGuid retrieves the current value of HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography\MachineGuid
// from the windows registry, to be used as a stable hardware identifier for windows
func currentMachineGuid(ctx context.Context, k types.Knapsack) (string, error) {
	parentKey, err := registry.OpenKey(registry.LOCAL_MACHINE, regKeyMachineGuidParent, registry.READ)
	if err != nil {
		return "", fmt.Errorf("opening registry key '%s': %w", regKeyMachineGuidParent, err)
	}

	defer func() {
		if err := parentKey.Close(); err != nil {
			k.Slogger().Log(ctx, slog.LevelInfo,
				"could not close registry key",
				"key_name", regKeyMachineGuidParent,
				"err", err,
			)
		}
	}()

	machGuid, _, err := parentKey.GetStringValue(regValueMachineGuid)
	if err != nil {
		return "", fmt.Errorf("reading registry value '%s' for key '%s': %w", regValueMachineGuid, regKeyMachineGuidParent, err)
	}

	return machGuid, nil
}
