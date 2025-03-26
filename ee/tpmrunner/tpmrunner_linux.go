//go:build linux
// +build linux

package tpmrunner

import (
	"errors"
	"os"
)

var terminalErrors = []error{
	// on linux, if the tpm device is not present, we get this error
	// stat /dev/tpm0: no such file or directory
	// we should check this when we create a new tpmrunner so this
	// is maybe belt and suspenders
	os.ErrNotExist,
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
