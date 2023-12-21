//go:build secure_enclave_test
// +build secure_enclave_test

package secureenclavesigner

import (
	"crypto/ecdsa"

	"github.com/kolide/krypto/pkg/echelper"
)

var ServerPubKeyDer string
var Undertest = false
var TestKey *ecdsa.PublicKey

func init() {
	if ServerPubKeyDer == "" {
		panic("ServerPubKeyDer must be set")
	}

	key, err := echelper.PublicB64DerToEcdsaKey([]byte(ServerPubKeyDer))
	if err != nil {
		panic(err)
	}

	TestKey = key
	Undertest = true
}
