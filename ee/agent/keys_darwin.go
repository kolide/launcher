//go:build darwin
// +build darwin

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/secureenclavesigner"
)

func setupHardwareKeys(slogger *slog.Logger, store types.GetterSetterDeleter) (keyInt, error) {

	// fetch any existing key data
	_, pubData, err := fetchKeyData(store)
	if err != nil {
		return nil, err
	}

	ses, err := secureenclavesigner.New(slogger, store)
	if err != nil {
		return nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	if pubData != nil {
		if err := json.Unmarshal(pubData, &ses); err != nil {
			// data is corrupt or not in the expected format, clear it
			slogger.Log(context.TODO(), slog.LevelError,
				"could not unmarshal stored key data, clearing key data and generating new keys",
				"err", err,
			)
			clearKeyData(slogger, store)

			ses, err = secureenclavesigner.New(slogger, store)
			if err != nil {
				return nil, fmt.Errorf("creating secureenclave signer: %w", err)
			}
		}
	}

	// this is kind of weird, but we need to call public to ensure the key is generated
	// it's done this way to do satisfying signer interface which doesn't return an error
	if ses.Public() == nil {
		return nil, errors.New("public key was not be created")
	}

	return ses, nil
}
