//go:build !secure_enclave_test
// +build !secure_enclave_test

package secureenclavesigner

import (
	"crypto/ecdsa"
)

const Undertest = false

// ServerPubKeyDer should never be set outside of testing.
// See test_keys.go.
var ServerPubKeyDer string

// TestKey should never be set outside of testing.
// See test_keys.go.
var TestKey *ecdsa.PublicKey
