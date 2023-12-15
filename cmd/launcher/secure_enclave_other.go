//go:build !darwin
// +build !darwin

package main

import (
	"fmt"
)

func runSecureEnclave(args []string) error {
	return fmt.Errorf("not implemented on non darwin platforms")
}
