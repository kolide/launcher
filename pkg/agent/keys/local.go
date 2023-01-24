package keys

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/echelper"
	"go.etcd.io/bbolt"
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

func SetupLocalDbKey(logger log.Logger, db *bbolt.DB) (*dbKey, error) {
	if key, err := fetchKey(db); key != nil && err == nil {
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
	if err := storeKey(db, key); err != nil {
		return nil, fmt.Errorf("storing new key: %w", err)
	}

	return &dbKey{key}, nil
}

func fetchKey(db *bbolt.DB) (*ecdsa.PrivateKey, error) {
	var raw []byte

	// There's nothing that can really return an error here. Either we have a key, or we don't.
	_ = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		raw = b.Get([]byte(localKey))
		return nil
	})

	// No key, just return nils
	if raw == nil {
		return nil, nil
	}

	return x509.ParseECPrivateKey(raw)
}

func storeKey(db *bbolt.DB, key *ecdsa.PrivateKey) error {
	raw, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling key: %w", err)
	}

	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		if err := b.Put([]byte(localKey), raw); err != nil {
			return err
		}

		return nil
	})
}
