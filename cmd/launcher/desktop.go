package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/ee/desktop"
	"github.com/kolide/launcher/ee/desktop/menu"
	"github.com/kolide/launcher/ee/desktop/server"
	"github.com/oklog/run"
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
	)

	if err := setFlags(*flagset, args); err != nil {
		return fmt.Errorf("setting flags: %w", err)
	}

	logger := logutil.NewServerLogger(env.Bool("LAUNCHER_DEBUG", false))
	logger = log.With(logger, "component", "desktop_process")
	level.Info(logger).Log("msg", "starting")

	shutdownChan := make(chan struct{})

	go handleSignals(logger, shutdownChan)
	go monitorParentProcess(logger, shutdownChan)

	var runGroup run.Group

	server, err := server.New(logger, *flauthtoken, desktop.DesktopSocketPath(os.Getpid()), shutdownChan)
	if err != nil {
		return err
	}

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

	// wait for shutdown
	runGroup.Add(func() error {
		<-shutdownChan
		return nil
	}, func(err error) {
		menu.Shutdown()
	})

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
	menu.Init(*flhostname)

	return nil
}

func handleSignals(logger log.Logger, signalReceivedChan chan<- struct{}) {
	signalsToHandle := []os.Signal{os.Interrupt, os.Kill}
	signals := make(chan os.Signal, len(signalsToHandle))
	signal.Notify(signals, signalsToHandle...)

	sig := <-signals

	level.Debug(logger).Log(
		"msg", "received signal",
		"signal", sig,
	)

	signalReceivedChan <- struct{}{}
}

// monitorParentProcess continuously checks to see if parent is a live and sends on provided channel if it is not
func monitorParentProcess(logger log.Logger, parentGoneChan chan<- struct{}) {
	ticker := time.NewTicker(2 * time.Second)

	for ; true; <-ticker.C {
		ppid := os.Getppid()

		if ppid <= 1 {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		exists, err := process.PidExistsWithContext(ctx, int32(ppid))

		// pretty sure this is not needed since it should call cancel on it's won when time is exceeded
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

	parentGoneChan <- struct{}{}
}

func setFlags(flagSet flag.FlagSet, args []string) error {
	err := flagSet.Parse(args)
	if err != nil {
		return err
	}

	flagSet.VisitAll(func(f *flag.Flag) {
		if f.Value.String() != "" {
			return
		}

		// look for env var
		if value, ok := os.LookupEnv(f.Name); ok {
			f.Value.Set(value)
		}
	})

	return nil
}
