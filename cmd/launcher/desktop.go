package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/desktop/menu"
	"github.com/kolide/launcher/ee/desktop/server"
	"github.com/oklog/run"
	"github.com/peterbourgon/ff/v3"
	"github.com/shirou/gopsutil/process"
)

func runDesktop(args []string) error {
	var (
		flagset    = flag.NewFlagSet("kolide desktop", flag.ExitOnError)
		flhostname = flagset.String(
			"hostname",
			"",
			"hostname launcher is connected to",
		)
		flauthtoken = flagset.String(
			"authtoken",
			"",
			"auth token for desktop server",
		)
		flsocketpath = flagset.String(
			"socket_path",
			"",
			"path to create socket",
		)
		flmenupath = flagset.String(
			"menu_path",
			"",
			"path to read menu data",
		)
		fldebug = flagset.Bool(
			"debug",
			false,
			"enable debug logging",
		)
	)

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	user, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	// set up logging
	logger := logutil.NewServerLogger(*fldebug)
	logger = log.With(logger,
		"subprocess", "desktop",
		"pid", os.Getpid(),
		"uid", user.Uid,
	)
	level.Info(logger).Log("msg", "starting")

	if *flsocketpath == "" {
		*flsocketpath = defaultSocketPath()
		level.Info(logger).Log(
			"msg", "using default socket path since none was provided",
			"socket_path", *flsocketpath,
		)
	}

	var runGroup run.Group

	// listen for signals
	runGroup.Add(func() error {
		listenSignals(logger)
		return nil
	}, func(error) {})

	// monitor parent
	runGroup.Add(func() error {
		monitorParentProcess(logger)
		return nil
	}, func(error) {})

	shutdownChan := make(chan struct{})
	server, err := server.New(logger, *flauthtoken, *flsocketpath, shutdownChan)
	if err != nil {
		return err
	}

	menu := menu.New(logger, *flhostname, *flmenupath)
	server.RegisterRefreshListener(func() {
		menu.Build()
	})

	// start desktop server
	runGroup.Add(server.Serve, func(err error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			level.Error(logger).Log(
				"msg", "shutting down server",
				"err", err,
			)
		}
	})

	// listen on shutdown channel
	runGroup.Add(func() error {
		<-shutdownChan
		return nil
	}, func(err error) {
		menu.Shutdown()
	})

	// run run group
	go func() {
		// have to run this in a goroutine because menu needs the main thread
		if err := runGroup.Run(); err != nil {
			level.Error(logger).Log(
				"msg", "running run group",
				"err", err,
			)
		}
	}()

	// blocks until shutdown called
	menu.Init()

	return nil
}

func listenSignals(logger log.Logger) {
	signalsToHandle := []os.Signal{os.Interrupt, os.Kill}
	signals := make(chan os.Signal, len(signalsToHandle))
	signal.Notify(signals, signalsToHandle...)

	sig := <-signals

	level.Debug(logger).Log(
		"msg", "received signal",
		"signal", sig,
	)
}

// monitorParentProcess continuously checks to see if parent is a live and sends on provided channel if it is not
func monitorParentProcess(logger log.Logger) {
	ticker := time.NewTicker(2 * time.Second)

	for ; true; <-ticker.C {
		ppid := os.Getppid()

		if ppid <= 1 {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		exists, err := process.PidExistsWithContext(ctx, int32(ppid))

		// pretty sure this `cancel()` call is not needed since it should call cancel on it's own when time is exceeded
		// https://cs.opensource.google/go/go/+/master:src/context/context.go;l=456?q=func%20WithDeadline&ss=go%2Fgo
		// but the linter and the context.WithTimeout docs say to do it
		cancel()
		if err != nil || !exists {
			level.Error(logger).Log(
				"msg", "parent process gone",
				"err", err,
			)
			break
		}
	}
}

func defaultSocketPath() string {
	const socketBaseName = "kolide_desktop.sock"

	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\%s_%d_%s`, socketBaseName, os.Getpid(), ulid.New())
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d", socketBaseName, os.Getpid()))
}
