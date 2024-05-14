//go:build !windows
// +build !windows

package watchdog

import (
	"context"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

func NewController(_ context.Context, _ types.Knapsack) (*WatchdogController, error) {
	return nil, nil
}

func (wc *WatchdogController) FlagsChanged(flagKeys ...keys.FlagKey) {
	return
}

func (wc *WatchdogController) ServiceEnabledChanged(enabled bool) {
	return
}

func (wc *WatchdogController) Run() error {
	return nil
}

func (wc *WatchdogController) Interrupt(_ error) {
	return
}
