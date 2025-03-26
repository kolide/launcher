//go:build windows
// +build windows

package tpmrunner

import (
	"errors"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpmutil/tbs"
)

var terminalErrors = []error{
	tbs.ErrTPMNotFound,

	// this covers the error "integrity check failed" we dont
	// believe a machine will recover from this
	tpm2.Error{
		Code: tpm2.RCIntegrity,
	},
}

// isTerminalTPMError returns true if we should stop trying to use the TPM.
func isTerminalTPMError(err error) bool {
	for _, e := range terminalErrors {
		if errors.Is(err, e) {
			return true
		}
	}

	return false
}
