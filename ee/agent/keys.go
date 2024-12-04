package agent

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type keyInt interface {
	crypto.Signer
	Type() string
}

var hardwareKeys keyInt = keys.Noop
var localDbKeys keyInt = keys.Noop

func HardwareKeys() keyInt {
	return hardwareKeys
}

func LocalDbKeys() keyInt {
	return localDbKeys
}

type secureEnclaveClient interface {
	CreateSecureEnclaveKey(uid string) (*ecdsa.PublicKey, error)
}

func SetupKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	slogger = slogger.With("component", "agentkeys")

	var err error

	// Always setup a local key
	localDbKeys, err = keys.SetupLocalDbKey(slogger, store)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	return nil
}
