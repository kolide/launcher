//go:build !secure_enclave_test
// +build !secure_enclave_test

package secureenclavesigner

// Undertest is true when running secure encalve test build
const Undertest = false

// TestServerPubKey should never be set outside of testing.
// See test_keys.go.
const TestServerPubKey = ""
