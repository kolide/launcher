//go:build !darwin
// +build !darwin

package main

import "errors"

func runSecureEnclave(args []string) error {
	return errors.New("not implemented on non darwin platforms")
}
