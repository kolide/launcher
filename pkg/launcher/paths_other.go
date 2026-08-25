//go:build !windows

package launcher

import "errors"

// Unreachable on non-Windows, included for compilation.
func runningElevated() (bool, error) {
	return false, errors.New("OS does not support elevation check")
}
