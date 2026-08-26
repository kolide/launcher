//go:build !windows

package currentprocess

import "errors"

// Unreachable on non-Windows, included for compilation.
func IsElevated() (bool, error) {
	return false, errors.New("OS does not support elevation check")
}
