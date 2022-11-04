package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/kolide/launcher/pkg/debug"
)

func runDebug(args []string) error {
	flagset := flag.NewFlagSet("debug", flag.ExitOnError)
	var (
		flRootDirectory = flagset.String(
			"root_directory",
			defaultRootDir(),
			"The location of the local database, pidfiles, etc.",
		)
	)

	if err := flagset.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *flRootDirectory == "" {
		return fmt.Errorf("--root_directory is required")
	}

	response, err := debug.ToggleDebug(*flRootDirectory)

	if err != nil {
		return fmt.Errorf("toggling debug server: %w", err)
	}

	fmt.Printf("%s\n", response)
	return nil
}

func defaultRootDir() string {
	possibleRootDirs := []string{
		"/var/kolide-k2/k2device.kolide.com",
		"/var/kolide-k2/k2device-preprod.kolide.com",
	}

	if runtime.GOOS == "windows" {
		possibleRootDirs = []string{
			`C:\Program Files\Kolide\Launcher-kolide-k2`,
		}
	}

	for _, possibleRootDir := range possibleRootDirs {
		if _, err := os.Stat(possibleRootDir); err == nil {
			return possibleRootDir
		}
	}

	return ""
}
