//go:build !darwin
// +build !darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/krypto/pkg/tpm"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

func setupHardwareKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter) (keyInt, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	priData, pubData, err := fetchKeyData(store)
	if err != nil {
		return nil, err
	}

	if pubData == nil || priData == nil {
		slogger.Log(context.TODO(), slog.LevelInfo,
			"generating new keys",
		)

		var err error
		priData, pubData, err = tpm.CreateKey()
		if err != nil {
			clearKeyData(slogger, store)
			traces.SetError(span, fmt.Errorf("creating key: %w", err))
			return nil, fmt.Errorf("creating key: %w", err)
		}

		span.AddEvent("new_key_created")

		if err := storeKeyData(store, priData, pubData); err != nil {
			clearKeyData(slogger, store)
			traces.SetError(span, fmt.Errorf("storing key: %w", err))
			return nil, fmt.Errorf("storing key: %w", err)
		}

		span.AddEvent("new_key_stored")
	}

	k, err := tpm.New(priData, pubData)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating tpm signer: from new key: %w", err))
		return nil, fmt.Errorf("creating tpm signer: from new key: %w", err)
	}

	return k, nil
}
