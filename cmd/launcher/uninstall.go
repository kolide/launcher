package main

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3"
)

// Specific to Unix platforms, matching only standard-looking identifiers
var identifierRegexp = regexp.MustCompile(`^\/var\/([-a-zA-Z0-9]*)\/.*\.kolide\.com`)

func runUninstall(args []string) error {
	var (
		flagset         = flag.NewFlagSet("kolide uninstaller", flag.ExitOnError)
		flRootDirectory = flagset.String("root_directory", "", "The location of the local database, pidfiles, etc.")
		_               = flagset.String(
			"config",
			"",
			"launcher flags configuration file",
		)
	)

	ffOpts := []ff.Option{
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithIgnoreUndefined(true),
		ff.WithEnvVarNoPrefix(),
	}

	if err := ff.Parse(flagset, args, ffOpts...); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	var identifier string
	matches := identifierRegexp.FindAllStringSubmatch(*flRootDirectory, -1)
	if len(matches) == 1 && len(matches[0]) == 2 {
		// Capture non-default identifiers if they match the regexp
		identifier = strings.TrimSpace(matches[0][1])
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	return removeLauncher(ctx, identifier)
}
