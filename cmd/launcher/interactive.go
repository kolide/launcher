package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
	"github.com/pkg/errors"
)

func interactive(args []string) error {
	flagset := flag.NewFlagSet("interactive", flag.ExitOnError)
	var (
		flOsquerydPath = flagset.String(
			"osqueryd_path",
			"",
			"The path to the oqueryd binary",
		)
		flSocketPath = flagset.String(
			"socket_path",
			env.String("SOCKET_PATH", filepath.Join(os.TempDir(), "osquery.sock")),
			"The path to the socket",
		)
	)

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

	if _, err := os.Stat(filepath.Dir(*flSocketPath)); os.IsNotExist(err) {
		if err := os.Mkdir(filepath.Dir(*flSocketPath), fs.DirMode); err != nil {
			return errors.Wrap(err, "creating socket path base directory")
		}
	}

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	fmt.Println(">> Starting osquery interactive with launcher tables")

	osqueryProc, err := os.StartProcess(osquerydPath, []string{
		"-S",
		fmt.Sprintf("--extensions_socket=%s", *flSocketPath),
	}, &pa)

	if err != nil {
		return fmt.Errorf("error starting osqueryd: %s", err)
	}

	extensionManagerServer, err := loadExtensions(*flSocketPath, osquerydPath)
	if err != nil {
		extensionManagerServer.Shutdown(context.Background())
		return fmt.Errorf("error loading extensions: %s", err)
	}

	// Wait until user exits the shell
	state, err := osqueryProc.Wait()
	if err != nil {
		return fmt.Errorf("error waiting for osqueryd: %s", err)
	}

	// Keep on keepin' on.
	fmt.Printf("<< Exited osquery interactive with launcher tables: %s\n", state.String())

	if err := extensionManagerServer.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("error shutting down extension manager: %s", err)
	}

	return nil
}

func loadExtensions(socketPath string, osquerydPath string) (*osquery.ExtensionManagerServer, error) {

	extensionManagerServer, err := osquery.NewExtensionManagerServer(
		"interactive",
		socketPath,
		osquery.ServerTimeout(10*time.Second),
	)

	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating extension manager server: %s", err)
	}

	client, err := osquery.NewClient(socketPath, 10*time.Second)
	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating osquery client: %s", err)
	}

	extensionManagerServer.RegisterPlugin(table.PlatformTables(client, log.NewNopLogger(), osquerydPath)...)
	extensionManagerServer.RegisterPlugin(table.LauncherTables(nil, nil)...)

	if err := extensionManagerServer.Start(); err != nil {
		return extensionManagerServer, errors.Wrap(err, "running extension server")
	}

	return extensionManagerServer, nil
}
