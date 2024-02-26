package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

	// were passing an empty array here just to get the default options
	cmdlineopts, err := launcher.ParseOptions("socket", make([]string, 0))
	if err != nil {
		return err
	}
	slogger := multislogger.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})).Logger
	fcOpts := []flags.Option{flags.WithCmdLineOpts(cmdlineopts)}
	flagController := flags.NewFlagController(slogger, inmemory.NewStore(), fcOpts...)
	k := knapsack.New(nil, flagController, nil, nil, nil)
	runner := runtime.New(k, opts...)
	go runner.Run()

	fmt.Println(*flPath)

	// Wait forever
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.Signal(15))
	<-sig

	// allow for graceful termination.
	runner.Shutdown()

	return nil
}
