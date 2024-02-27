//go:build !darwin
// +build !darwin

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/krypto/pkg/tpm"
	"github.com/kolide/launcher/ee/agent/types"
)

// nolint:unused
func setupHardwareKeys(slogger *slog.Logger, store types.GetterSetterDeleter) (keyInt, error) {
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
			return nil, fmt.Errorf("creating key: %w", err)
		}

		if err := storeKeyData(store, priData, pubData); err != nil {
			clearKeyData(slogger, store)
			return nil, fmt.Errorf("storing key: %w", err)
		}
	}

	k, err := tpm.New(priData, pubData)
	if err != nil {
		return nil, fmt.Errorf("creating tpm signer: from new key: %w", err)
	}

	return k, nil
}
