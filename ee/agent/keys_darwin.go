//go:build darwin
// +build darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/secureenclaverunner"
)

// SetHardwareKeysRunner creates a secure enclave runner and sets it as the agent hardware key as it also implements the keyInt/crypto.Signer interface.
// The returned execute and interrupt functions can be used to start and stop the secure enclave runner, generally via a run group.
func SetHardwareKeysRunner(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, secureEnclaveClient secureEnclaveClient) (execute func() error, interrupt func(error), err error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	ser, err := secureenclaverunner.New(ctx, slogger, store, secureEnclaveClient)
	if err != nil {
		observability.SetError(span, fmt.Errorf("creating secureenclave signer: %w", err))
		return nil, nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	hardwareKeys = ser
	return ser.Execute, ser.Interrupt, nil
}
