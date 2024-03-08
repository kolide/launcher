//go:build darwin
// +build darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/kolide/launcher/pkg/traces"
)

func setupHardwareKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter) (keyInt, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ses, err := secureenclavesigner.New(ctx, slogger, store)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating secureenclave signer: %w", err))
		return nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	return ses, nil
}
