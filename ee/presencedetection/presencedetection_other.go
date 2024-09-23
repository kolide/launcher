//go:build !windows
// +build !windows

package presencedetection

import "errors"

func Detect(reason string) (bool, error) {
	return false, errors.New("not implemented")
}
