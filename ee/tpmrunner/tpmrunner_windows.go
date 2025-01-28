//go:build windows
// +build windows

package tpmrunner

import (
	"errors"

	"github.com/google/go-tpm/tpmutil/tbs"
)

func isTPMNotFoundErr(err error) bool {
	return errors.Is(err, tbs.ErrTPMNotFound)
}
