package agent

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

type keyInt interface {
	crypto.Signer
	Type() string
}

var hardwareKeys keyInt = keys.Noop
var localDbKeys keyInt = keys.Noop

// HardwareKeys returns the hardware keys for the agent, it's critical to not cache this value as it may change during runtime.
func HardwareKeys() keyInt {
	return hardwareKeys
}

func LocalDbKeys() keyInt {
	return localDbKeys
}

type secureEnclaveClient interface {
	CreateSecureEnclaveKey(ctx context.Context, uid string) (*ecdsa.PublicKey, error)
	VerifySecureEnclaveKey(ctx context.Context, uid string, pubKey *ecdsa.PublicKey) (bool, error)
}

func SetupKeys(_ context.Context, slogger *slog.Logger, store types.GetterSetterDeleter) error {
	slogger = slogger.With("component", "agentkeys")

	var err error

	// Always setup a local key
	localDbKeys, err = keys.SetupLocalDbKey(slogger, store)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	return nil
}
