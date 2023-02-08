package keys

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent"
)

// This duplicates some of pkg/osquery/extension.go but that feels like the wrong place.
// Really, we should have a simpler interface over a storage layer.
const (
	bucketName = "config"
	localKey   = "localEccKey"
)

// dbKey is keyInt over a key stored in the agent database. Its used in places where we don't want, or don't have, the hardware key.
type dbKey struct {
	*ecdsa.PrivateKey
}

func (k dbKey) Type() string {
	return "local"
}

func SetupLocalDbKey(logger log.Logger, getset GetterSetter) (*dbKey, error) {
	if key, err := fetchKey(getset); key != nil && err == nil {
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
	if err := storeKey(getset, key); err != nil {
		return nil, fmt.Errorf("storing new key: %w", err)
	}

	return &dbKey{key}, nil
}

func fetchKey(getter agent.Getter) (*ecdsa.PrivateKey, error) {
	raw, err := getter.Get(localKey)
	// var raw []byte

	// // There's nothing that can really return an error here. Either we have a key, or we don't.
	// _ = db.View(func(tx *bbolt.Tx) error {
	// 	b := tx.Bucket([]byte(bucketName))
	// 	if b == nil {
	// 		return nil
	// 	}

	// 	raw = b.Get([]byte(localKey))
	// 	return nil
	// })

	// // No key, just return nils
	// if raw == nil {
	// 	return nil, nil
	// }

	return x509.ParseECPrivateKey(raw)
}

func storeKey(setter agent.Setter, key *ecdsa.PrivateKey) error {
	raw, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling key: %w", err)
	}

	return setter.Set([]byte(localKey), raw)
}
