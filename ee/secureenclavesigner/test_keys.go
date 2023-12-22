//go:build secure_enclave_test
// +build secure_enclave_test

package secureenclavesigner

// Using ldflags to set the pub key and using build tag.
//
// This kind of feels like belt and suspenders.
//
// However, a non -ldflag path other than hard coding a test private key (gross),
// has not been discovered.
//
// We could probably drop the build tag and just use the -ldflag, then determine
// if we're under test by checking the value of the var set by the -ldflag, but
// that feels more tangly.

// Undertest is true when running secure enclave test build
const Undertest = true

// TestServerPubKey is the public key of the server in DER format
// when building the binary for testing, we set this with -ldflags
// so the wrapper test can sign requests with the private portion
// of the key it used to set this value.
// See secureenclavesigner_test.go
var TestServerPubKey string
