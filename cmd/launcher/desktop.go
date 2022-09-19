package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"time"

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
	)

	if err := flagset.Parse(args); err != nil {
		return err
	}

	shutdownChan := make(chan struct{})

	go handleSignals(shutdownChan)
	go monitorParentProcess(shutdownChan)

	var runGroup run.Group

	server, err := server.New(shutdownChan)
	if err != nil {
		return err
	}

	// start desktop server
	runGroup.Add(server.Serve, func(err error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			//TODO: log this
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
			//TODO: log this
		}
	}()

	// blocks until shutdown called
	menu.Init(*flhostname)

	return nil
}

func handleSignals(signalReceivedChan chan<- struct{}) {
	signalsToHandle := []os.Signal{os.Interrupt, os.Kill}
	signals := make(chan os.Signal, len(signalsToHandle))
	signal.Notify(signals, signalsToHandle...)

	<-signals

	//TODO: log signal
	signalReceivedChan <- struct{}{}
}

// monitorParentProcess continuously checks to see if parent is a live and sends on provided channel if it is not
func monitorParentProcess(parentGoneChan chan<- struct{}) {
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
			//TODO: log parent gone
			break
		}
	}

	parentGoneChan <- struct{}{}
}
