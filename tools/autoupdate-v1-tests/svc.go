// +build !windows

package main

import "errors"

func runWindowsSvc(args []string) error {
	return errors.New("This isn't windows")
}

func runWindowsSvcForeground(args []string) error {
	return errors.New("This isn't windows")
}

func runInstallService(args []string) error {
	return errors.New("This isn't windows")
}

func runRemoveService(args []string) error {
	return errors.New("This isn't windows")
}
