//go:build linux
// +build linux

package tpmrunner

// isTPMNotFoundErr always return false on linux because we don't yet how to
// detect if a TPM is not found on linux.
func isTPMNotFoundErr(err error) bool {
	return false
}
