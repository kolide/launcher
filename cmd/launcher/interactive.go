package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/kolide/launcher/pkg/osquery/interactive"
	"github.com/pkg/errors"
)

func runInteractive(args []string) error {

	flagset := flag.NewFlagSet("interactive", flag.ExitOnError)
	var (
		flOsquerydPath = flagset.String(
			"osqueryd_path",
			"",
			"The path to the oqueryd binary",
		)
		flOsqueryFlags arrayFlags
	)

	flagset.Var(&flOsqueryFlags, "osquery_flag", "Flags to pass to osquery (possibly overriding Launcher defaults)")

	flagset.Usage = commandUsage(flagset, "interactive")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	osquerydPath := *flOsquerydPath
	if osquerydPath == "" {
		osquerydPath = findOsquery()
		if osquerydPath == "" {
			return errors.New("Could not find osqueryd binary")
		}
	}

	rootDir, err := os.MkdirTemp("", "launcher-osquery-interactive")
	if err != nil {
		return errors.Wrap(err, "creating temp dir for interactive mode")
	}

	defer func() {
		if err := os.RemoveAll(rootDir); err != nil {
			fmt.Printf("error removing launcher interactive temp dir: %s\n", err)
		}
	}()

	osqueryProc, extensionsServer, err := interactive.StartProcess(rootDir, osquerydPath, flOsqueryFlags)
	if err != nil {
		return fmt.Errorf("error starting osqueryd: %s", err)
	}
	defer extensionsServer.Shutdown(context.Background())

	// Wait until user exits the shell
	_, err = osqueryProc.Wait()
	if err != nil {
		return fmt.Errorf("error waiting for osqueryd: %s", err)
	}

	return nil
}
