// +build !windows

package main

import (
	"fmt"
	"os"
)

func runWindowsSvc(args []string) error {
	fmt.Println("This isn't windows")
	os.Exit(1)
	return nil
}

func runWindowsSvcDebug(args []string) error {
	fmt.Println("This isn't windows")
	os.Exit(1)
	return nil
}
