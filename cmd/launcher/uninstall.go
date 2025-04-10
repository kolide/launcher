package main

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
)

// Specific to Unix platforms, matching only standard-looking identifiers
var identifierRegexp = regexp.MustCompile(`^\/var\/([-a-zA-Z0-9]*)\/.*\.kolide\.com`)

func runUninstall(_ *multislogger.MultiSlogger, args []string) error {
	var (
		flagset          = flag.NewFlagSet("kolide uninstaller", flag.ExitOnError)
		flRootDirectory  = flagset.String("root_directory", "", "The location of the local database, pidfiles, etc.")
		flConfigFilePath = flagset.String("config", "", "config file to parse options from (optional)")
	)
	flagset.Usage = commandUsage(flagset, "launcher uninstall")

	if err := ff.Parse(flagset, args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	launcherOptions := make([]string, 0)
	if *flRootDirectory != "" {
		launcherOptions = append(launcherOptions, "-root_directory", *flRootDirectory)
	}
	if *flConfigFilePath != "" {
		launcherOptions = append(launcherOptions, "-config", *flConfigFilePath)
	}

	opts, err := launcher.ParseOptions("uninstall", launcherOptions)
	if err != nil {
		return fmt.Errorf("parsing launcher options: %w", err)
	}

	var identifier string
	matches := identifierRegexp.FindAllStringSubmatch(opts.RootDirectory, -1)
	if len(matches) == 1 && len(matches[0]) == 2 {
		// Capture non-default identifiers if they match the regexp
		identifier = strings.TrimSpace(matches[0][1])
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	return removeLauncher(ctx, identifier)
}
