//go:build darwin
// +build darwin

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/kolide/launcher/ee/secureenclavesigner"
)

// runSecureEnclave performs either a create-key operation using the secure enclave.
// It's available as a separate command because launcher runs as root by default and since it's
// not in a user security context, it can't use the secure enclave directly. However, this command
// can be run in the user context using launchctl.
func runSecureEnclave(args []string) error {
	// currently we are just creating key, but plan to add sign command in future
	if len(args) < 1 {
		return errors.New("not enough arguments, expect create_key")
	}

	switch args[0] {
	case secureenclavesigner.CreateKeyCmd:
		return createSecureEnclaveKey()

	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func createSecureEnclaveKey() error {
	secureEnclavePubKey, err := secureenclave.CreateKey()
	if err != nil {
		return fmt.Errorf("creating secure enclave key: %w", err)
	}

	secureEnclavePubDer, err := echelper.PublicEcdsaToB64Der(secureEnclavePubKey)
	if err != nil {
		return fmt.Errorf("marshalling public key to der: %w", err)
	}

	os.Stdout.Write(secureEnclavePubDer)
	return nil
}
