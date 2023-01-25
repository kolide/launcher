//go:build darwin
// +build darwin

package agent

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/secureenclave"
	"go.etcd.io/bbolt"
)

func setupHardwareKeys(logger log.Logger, db *bbolt.DB) (keyInt, error) {
	_, pubData, err := fetchKeyData(db)
	if err != nil {
		return nil, err
	}

	if pubData == nil {
		level.Info(logger).Log("msg", "Generating new keys")

		var err error
		pubData, err = secureenclave.CreateKey()
		if err != nil {
			return nil, fmt.Errorf("creating key: %w", err)
		}

		if err := storeKeyData(db, nil, pubData); err != nil {
			clearKeyData(logger, db)
			return nil, fmt.Errorf("storing key: %w", err)
		}
	}

	k, err := secureenclave.New(pubData)
	if err != nil {
		return nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	return k, nil
}
