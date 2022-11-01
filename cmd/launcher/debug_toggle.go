//go:build !windows
// +build !windows

package main

import (
	"fmt"

	"github.com/kolide/launcher/pkg/debug"
)

func runDebugServer([]string) error {
	returm fmt.Errorf("not implemented")
}
