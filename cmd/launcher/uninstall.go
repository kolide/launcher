package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kolide/kit/logutil"
	"github.com/peterbourgon/ff/v3"
	"github.com/pkg/errors"
)

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

	logger := logutil.NewCLILogger(true)

	// Attempt to get the identifier from the config file if one was provided
	identifier, err := getIdentifierFromConfigFile(*flconfig)
	if err != nil {
		return err
	}

	return removeLauncher(context.Background(), logger, identifier)
}

func getIdentifierFromConfigFile(configFilePath string) (string, error) {
	if configFilePath == "" {
		return "", nil
	}

	f, err := os.Open(configFilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open config file '%s'.", configFilePath)
	}
	defer f.Close()

	var identifier string
	prefix := "root_directory "
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Find the root_directory line
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		// Parse out the identifier part of the path
		substrs := strings.Split(strings.TrimSpace(strings.TrimPrefix(line, prefix)), string(os.PathSeparator))

		if len(substrs) >= 3 {
			identifier = strings.TrimSpace(substrs[2])
			break
		}
	}

	return identifier, nil
}
