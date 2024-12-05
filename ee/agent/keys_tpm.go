//go:build !darwin
// +build !darwin

package agent

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tpmrunner"
)

// SetHardwareKeysRunner creates a tpm runner and sets it as the agent hardware key as it also implements the keyInt/cyrpto.Signer interface.
// The returned execute and interrupt functions can be used to start and stop the secure enclave runner, generally via a run group.
func SetHardwareKeysRunner(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, _ secureEnclaveClient) (execute func() error, interrupt func(error), err error) {
	tpmRunner, err := tpmrunner.New(ctx, slogger, store)
	if err != nil {
		return nil, nil, err
	}

	hardwareKeys = tpmRunner
	return tpmRunner.Execute, tpmRunner.Interrupt, nil
}
