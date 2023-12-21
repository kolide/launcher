//go:build !secure_enclave_test
// +build !secure_enclave_test

package secureenclavesigner

import (
	"crypto/ecdsa"
)

var ServerPubKeyDer string
var Undertest = false
var TestKey *ecdsa.PublicKey
