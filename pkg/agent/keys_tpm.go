//go:build !darwin
// +build !darwin

package agent

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/tpm"
	"go.etcd.io/bbolt"
)

func setupHardwareKeys(logger log.Logger, db *bbolt.DB) (keyInt, error) {
	priData, pubData, err := fetchKeyData(db)
	if err != nil {
		return nil, err
	}

	if pubData == nil || priData == nil {
		level.Info(logger).Log("Generating new keys")

		var err error
		priData, pubData, err = tpm.CreateKey()
		if err != nil {
			clearKeyData(logger, db)
			return nil, fmt.Errorf("creating key: %w", err)
		}

		if err := storeKeyData(db, priData, pubData); err != nil {
			clearKeyData(logger, db)
			return nil, fmt.Errorf("storing key: %w", err)
		}
	}

	k, err := tpm.New(priData, pubData)
	if err != nil {
		return nil, fmt.Errorf("creating tpm signer: from new key: %w", err)
	}

	return k, nil
}
