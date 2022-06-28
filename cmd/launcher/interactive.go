package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
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

	if _, err := os.Stat(filepath.Dir(*flSocketPath)); os.IsNotExist(err) {
		if err := os.Mkdir(filepath.Dir(*flSocketPath), fs.DirMode); err != nil {
			return errors.Wrap(err, "creating socket path base directory")
		}
	}

	opts := []runtime.OsqueryInstanceOption{
		runtime.WithExtensionSocketPath(*flSocketPath),
		runtime.WithOsqueryExtensionPlugins(table.LauncherTables(nil, nil)...),
	}

	runner, err := runtime.LaunchInstance(opts...)
	if err != nil {
		return errors.Wrap(err, "creating osquery instance")
	}

	fmt.Println(*flSocketPath)

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	fmt.Println(">> Starting launcher osquery interactive shell")
	proc, err := os.StartProcess(*flOsquerydPath, []string{"-S", "--connect", *flSocketPath}, &pa)
	if err != nil {
		panic(err)
	}

	// Wait until user exits the shell
	state, err := proc.Wait()
	if err != nil {
		panic(err)
	}

	// Keep on keepin' on.
	fmt.Printf("<< Exited launcher osquery interactive shell: %s\n", state.String())

	// allow for graceful termination.
	runner.Shutdown()

	return nil
}
