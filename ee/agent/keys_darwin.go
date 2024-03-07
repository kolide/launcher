//go:build darwin
// +build darwin

package agent

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/secureenclavesigner"
)

func setupHardwareKeys(slogger *slog.Logger, store types.GetterSetterDeleter) (keyInt, error) {
	ses, err := secureenclavesigner.New(slogger, store)
	if err != nil {
		return nil, fmt.Errorf("creating secureenclave signer: %w", err)
	}

	// this is kind of weird, but we need to call public to ensure the key is generated
	// it's done this way to do satisfying signer interface which doesn't return an error
	if ses.Public() == nil {
		return nil, errors.New("public key was not be created")
	}

	return ses, nil
}
