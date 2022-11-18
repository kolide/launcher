package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/osquery/interactive"
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
		osquerydPath = autoupdate.FindNewest(context.Background(), osquerydPath)
	}

	// have to keep tempdir name short so we don't exceed socket length
	rootDir, err := os.MkdirTemp("", "launcher-interactive")
	if err != nil {
		return fmt.Errorf("creating temp dir for interactive mode: %w", err)
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
