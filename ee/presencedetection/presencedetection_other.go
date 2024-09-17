//go:build !darwin
// +build !darwin

package presencedetection

import "errors"

func Detect(reason string) (bool, error) {
	// Implement detection logic for non-Darwin platforms
	return false, errors.New("detection not implemented for this platform")
}
