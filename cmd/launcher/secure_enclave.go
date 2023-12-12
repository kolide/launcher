package main

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
)

func runSecureEnclave(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("not enough arguments, expect create_key or sign")
	}

	switch args[0] {
	case "create-key":
		return createSecureEnclaveKey()

	case "sign":
		if len(args) < 2 {
			return fmt.Errorf("not enough arguments, expect sign <data>")
		}
		return signWithSecureEnclave(args[1])

	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func createSecureEnclaveKey() error {
	key, err := secureenclave.CreateKey()
	if err != nil {
		return fmt.Errorf("creating secure enclave key: %w", err)
	}

	seSigner, err := secureenclave.New(key)
	if err != nil {
		return fmt.Errorf("creating secure enclave signer: %w", err)
	}

	b65Der, err := echelper.PublicEcdsaToB64Der(seSigner.Public().(*ecdsa.PublicKey))
	if err != nil {
		return fmt.Errorf("converting public key to b64 der: %w", err)
	}

	// write der to stdout
	fmt.Print(string(b65Der))
	return nil
}

func signWithSecureEnclave(request string) error {
	return fmt.Errorf("not implemented")
}
