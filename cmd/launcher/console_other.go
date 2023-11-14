//go:build !windows
// +build !windows

package main

func attachConsole() error {
	return nil
}

func detachConsole() error {
	return nil
}
