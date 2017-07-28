package main

import (
	"github.com/kolide/kit/version"
)

func runVersion(args []string) error {
	version.PrintFull()
	return nil
}
