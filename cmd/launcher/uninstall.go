package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3"
)

var rootDirRegexp = regexp.MustCompile(`^root_directory \/var\/([-a-zA-Z0-9]*)\/.*\.kolide\.com`)

func runUninstall(args []string) error {
	var (
		flagset  = flag.NewFlagSet("kolide uninstaller", flag.ExitOnError)
		flconfig = flagset.String(
			"config",
			"",
			"launcher flags configuration file",
		)
	)

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Attempt to get the identifier from the config file if one was provided
	identifier, err := getIdentifierFromConfigFile(*flconfig)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	return removeLauncher(ctx, identifier)
}

func getIdentifierFromConfigFile(configFilePath string) (string, error) {
	if configFilePath == "" {
		return "", nil
	}

	f, err := os.Open(configFilePath)
	if err != nil {
		return "", fmt.Errorf("opening config file '%s': %w", configFilePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		matches := rootDirRegexp.FindAllStringSubmatch(line, -1)
		if !(len(matches) == 1 && len(matches[0]) == 2) {
			// Skip lines that do not match regexp
			continue
		}

		return strings.TrimSpace(matches[0][1]), nil
	}

	return "", nil
}
