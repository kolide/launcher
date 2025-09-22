//go:build !windows
// +build !windows

package watchdog

import (
	"context"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

type WatchdogController struct{}

func NewController(_ context.Context, _ types.Knapsack, _ string) (*WatchdogController, error) {
	return nil, nil
}

func (wc *WatchdogController) FlagsChanged(_ context.Context, flagKeys ...keys.FlagKey) {}

func (wc *WatchdogController) ServiceEnabledChanged(enabled bool) {}

func (wc *WatchdogController) Run() error {
	return nil
}

func (wc *WatchdogController) Interrupt(_ error) {}
