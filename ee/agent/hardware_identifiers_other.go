//go:build !windows
// +build !windows

package agent

import (
	"context"

	"github.com/kolide/launcher/ee/agent/types"
)

// currentMachineGuid is only implemented for windows, where the hardware_uuid
// cannot be used as a stable identifier
func currentMachineGuid(_ context.Context, _ types.Knapsack) (string, error) {
	return "", nil
}
