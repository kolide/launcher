//go:build darwin
// +build darwin

package agent

import (
	"errors"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
)

// nolint:unused
func setupHardwareKeys(logger log.Logger, store types.GetterSetterDeleter) (keyInt, error) {
	// We're seeing issues where launcher hangs (and does not complete startup) on the
	// Sonoma Beta 2 release when trying to interact with the secure enclave below, on
	// CreateKey. Since we don't expect this to work at the moment anyway, we are
	// short-circuiting and returning early for now.
	return nil, errors.New("secure enclave is not currently supported")

	/*
		_, pubData, err := fetchKeyData(store)
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

			if err := storeKeyData(store, nil, pubData); err != nil {
				clearKeyData(logger, store)
				return nil, fmt.Errorf("storing key: %w", err)
			}
		}

		k, err := secureenclave.New(pubData)
		if err != nil {
			return nil, fmt.Errorf("creating secureenclave signer: %w", err)
		}

		return k, nil
	*/
}
