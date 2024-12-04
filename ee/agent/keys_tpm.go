//go:build !darwin
// +build !darwin

package agent

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tpmrunner"
)

func SetupHardwareKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, _ secureEnclaveClient) (execute func() error, interrupt func(error), err error) {
	tpmRunner, err := tpmrunner.New(ctx, slogger, store)
	if err != nil {
		return nil, nil, err
	}

	hardwareKeys = tpmRunner
	return tpmRunner.Execute, tpmRunner.Interrupt, nil
}
