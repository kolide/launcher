//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"os"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runWindowsSvc(_ *multislogger.MultiSlogger, _ []string) error {
	fmt.Println("This isn't windows")
	os.Exit(1)
	return nil
}

func runWindowsSvcForeground(_ *multislogger.MultiSlogger, _ []string) error {
	fmt.Println("This isn't windows")
	os.Exit(1)
	return nil
}
