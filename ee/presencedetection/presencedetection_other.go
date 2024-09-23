//go:build !windows
// +build !windows

package presencedetection

import "errors"

func Register(credentialName string) error {
	return errors.New("not implemented")
}

func Detect(reason string, credentialName string) (bool, error) {
	return false, errors.New("not implemented")
}
