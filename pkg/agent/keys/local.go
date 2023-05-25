package keys

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent/types"
)

// This duplicates some of pkg/osquery/extension.go but that feels like the wrong place.
// Really, we should have a simpler interface over a storage layer.
const (
	localKey = "localEccKey"
)

// dbKey is keyInt over a key stored in the agent database. Its used in places where we don't want, or don't have, the hardware key.
type dbKey struct {
	*ecdsa.PrivateKey
}

func (k dbKey) Type() string {
	return "local"
}

func SetupLocalDbKey(logger log.Logger, store types.GetterSetter) (*dbKey, error) {
	if key, err := fetchKey(store); key != nil && err == nil {
		level.Info(logger).Log("msg", "found local key in database")
		return &dbKey{key}, nil
	} else if err != nil {
		level.Info(logger).Log("msg", "Failed to parse key, regenerating", "err", err)
	} else if key == nil {
		level.Info(logger).Log("msg", "No key found, generating new key")
	}

	// Time to regenerate!
	key, err := echelper.GenerateEcdsaKey()
	if err != nil {
		return nil, fmt.Errorf("generating new key: %w", err)
	}

	// Store the key in the database.
	if err := storeKey(store, key); err != nil {
		return nil, fmt.Errorf("storing new key: %w", err)
	}

	return &dbKey{key}, nil
}

func fetchKey(store types.Getter) (*ecdsa.PrivateKey, error) {
	raw, _ := store.Get([]byte(localKey))
	if raw == nil {
		return nil, nil
	}
	return x509.ParseECPrivateKey(raw)
}

func storeKey(setter types.Setter, key *ecdsa.PrivateKey) error {
	raw, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling key: %w", err)
	}

	return setter.Set([]byte(localKey), raw)
}
