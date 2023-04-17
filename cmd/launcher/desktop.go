package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/desktop/menu"
	"github.com/kolide/launcher/ee/desktop/notify"
	"github.com/kolide/launcher/ee/desktop/server"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/task"
	"github.com/oklog/run"
	"github.com/peterbourgon/ff/v3"
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
		// flmonitorurl = flagset.String(
		// 	"monitor_url",
		// 	"",
		// 	"url to monitor parent",
		// )
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
		flIconPath = flagset.String(
			"icon_path",
			"",
			"path to icon file",
		)
	)

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// set up logging
	logger := logutil.NewServerLogger(*fldebug)
	logger = log.With(logger,
		"subprocess", "desktop",
		"pid", os.Getpid(),
	)

	// Try to get the current user, so we can use the UID for logging. Not a fatal error if we can't, though
	user, err := user.Current()
	if err != nil {
		level.Debug(logger).Log(
			"msg", "error getting current user",
			"err", err,
		)
	} else {
		logger = log.With(logger,
			"uid", user.Uid,
		)
	}

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

	// Set up notification sending and listening
	notifier := notify.NewDesktopNotifier(logger, *flIconPath)
	runGroup.Add(notifier.Listen, notifier.Interrupt)

	// monitor parent
	// runGroup.Add(func() error {
	// 	monitorParentProcess(logger, *flmonitorurl, 2*time.Second)
	// 	return nil
	// }, func(error) {})

	shutdownChan := make(chan struct{})
	server, err := server.New(logger, *flauthtoken, *flsocketpath, shutdownChan, notifier)
	if err != nil {
		return err
	}

	m := menu.New(logger, *flhostname, *flmenupath)
	refreshMenu := func() {
		m.Build()
	}
	server.RegisterRefreshListener(refreshMenu)

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
		m.Shutdown()
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
	m.Init()

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
func monitorParentProcess(logger log.Logger, monitorUrl string, interval time.Duration) {
	monitorTask := task.New(
		"desktop-monitor",
		task.Repeats(),
		task.WithInterval(interval))
	defer monitorTask.Stop()

	client := http.Client{
		Timeout: interval,
	}

	const maxErrCount = 3
	errCount := 0

	for ; true; <-monitorTask.C() {
		response, err := client.Get(monitorUrl)

		if response != nil {
			// This is the secret sauce to reusing a single connection, you have to read the body in full
			// before closing, otherwise a new connection is established each time.
			// thank you Chris Bao! this article explains this well
			// https://organicprogrammer.com/2021/10/25/understand-http1-1-persistent-connection-golang/
			// in our case the monitor server spun up by desktop_runner does not write to the body so this is not strictly nessessary, but doesn't hurt
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}

		// no error, 200 response
		if err == nil && response != nil && response.StatusCode == 200 {
			errCount = 0
			continue
		}

		// have an error or bad status code
		errCount++

		// retry
		if errCount < maxErrCount {
			level.Debug(logger).Log(
				"msg", "could not connect to parent, will retry",
				"err", err,
				"attempts", errCount,
				"max_attempts", maxErrCount,
			)

			continue
		}

		// errCount => maxErrCount, exit
		level.Debug(logger).Log(
			"msg", "could not connect to parent, max attempts reached, exiting",
			"err", err,
			"attempts", errCount,
			"max_attempts", maxErrCount,
		)

		break
	}
}

func defaultSocketPath() string {
	const socketBaseName = "kolide_desktop.sock"

	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\%s_%d_%s`, socketBaseName, os.Getpid(), ulid.New())
	}

	return agent.TempPath(fmt.Sprintf("%s_%d", socketBaseName, os.Getpid()))
}
