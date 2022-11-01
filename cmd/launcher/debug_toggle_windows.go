//go:build windows
// +build windows

package main

import (
	"fmt"

	"github.com/kolide/launcher/pkg/debug"
)

func runDebug([]string) error {
	response, err := debug.ToggleDebugServer()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", response)
	return nil
}
