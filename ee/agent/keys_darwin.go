//go:build darwin
// +build darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/secureenclaverunner"
	"github.com/kolide/launcher/pkg/traces"
)

func SetupHardwareKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, secureEnclaveClient secureEnclaveClient) (execute func() error, interrupt func(error), err error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ser, err := secureenclaverunner.New(ctx, slogger, store, secureEnclaveClient)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating secureenclave signer: %w", err))
		return nil, nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	hardwareKeys = ser
	return ser.Execute, ser.Interrupt, nil
}
