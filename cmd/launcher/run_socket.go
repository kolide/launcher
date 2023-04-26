package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
)

func runSocket(args []string) error {
	flagset := flag.NewFlagSet("launcher socket", flag.ExitOnError)
	var (
		flPath = flagset.String(
			"path",
			env.String("SOCKET_PATH", agent.TempPath("osquery.sock")),
			"The path to the socket",
		)
		flLauncherTables = flagset.Bool(
			"launcher-tables",
			false,
			"Run with launcher specific tables",
		)
	)
	flagset.Usage = commandUsage(flagset, "launcher socket")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Dir(*flPath)); os.IsNotExist(err) {
		if err := os.Mkdir(filepath.Dir(*flPath), fsutil.DirMode); err != nil {
			return fmt.Errorf("creating socket path base directory: %w", err)
		}
	}

	opts := []runtime.OsqueryInstanceOption{
		runtime.WithExtensionSocketPath(*flPath),
	}

	if *flLauncherTables {
		opts = append(opts, runtime.WithOsqueryExtensionPlugins(table.LauncherTables(nil)...))
	}

	runner, err := runtime.LaunchInstance(opts...)
	if err != nil {
		return fmt.Errorf("creating osquery instance: %w", err)
	}

	fmt.Println(*flPath)

	// Wait forever
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill, syscall.Signal(15))
	<-sig

	// allow for graceful termination.
	runner.Shutdown()

	return nil
}
