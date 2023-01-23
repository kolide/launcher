//go:build darwin
// +build darwin

package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/secureenclave"
	"go.etcd.io/bbolt"
)

func setupHardwareKeys(logger log.Logger, db *bbolt.DB) error {
	_, pubData, err := fetchKeyData(db)
	if err != nil {
		return err
	}

	if pubData == nil {
		level.Info(logger).Log("Generating new keys")
		pub, err := secureenclave.CreateKey()
		if err != nil {
			return fmt.Errorf("creating key: %w", err)
		}

		if err := storeKeyData(db, nil, ecdsaToRaw(pub)); err != nil {
			return fmt.Errorf("storing key: %w", err)
		}

		k, err := secureenclave.New(*pub)
		if err != nil {
			return fmt.Errorf("creating secureenclave signer:, from new key %w", err)
		}

		Keys = k
		return nil
	}

	k, err := secureenclave.New(*rawToEcdsa(pubData))
	if err != nil {
		return fmt.Errorf("creating secureenclave signer: %w", err)
	}

	Keys = k
	return nil
}

// TODO: These raw functions should just move into secureenclave. There's some skew between Create and New

func rawToEcdsa(raw []byte) *ecdsa.PublicKey {
	ecKey := new(ecdsa.PublicKey)
	ecKey.Curve = elliptic.P256()
	ecKey.X, ecKey.Y = elliptic.Unmarshal(ecKey.Curve, raw)
	return ecKey
}

func ecdsaToRaw(key *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(elliptic.P256(), key.X, key.Y)
}
