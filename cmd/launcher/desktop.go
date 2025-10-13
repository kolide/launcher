package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strconv"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent"
	runnerserver "github.com/kolide/launcher/ee/desktop/runner/server"
	"github.com/kolide/launcher/ee/desktop/user/menu"
	"github.com/kolide/launcher/ee/desktop/user/notify"
	userserver "github.com/kolide/launcher/ee/desktop/user/server"
	"github.com/kolide/launcher/ee/desktop/user/universallink"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/authedclient"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/rungroup"
	"github.com/kolide/systray"
	"github.com/peterbourgon/ff/v3"
)

func runDesktop(_ *multislogger.MultiSlogger, args []string) error {
	var (
		flagset    = flag.NewFlagSet("kolide desktop", flag.ExitOnError)
		flhostname = flagset.String(
			"hostname",
			"",
			"hostname launcher is connected to",
		)
		flUserServerAuthToken = flagset.String(
			"user_server_auth_token",
			"",
			"auth token for user server",
		)
		flUserServerSocketPath = flagset.String(
			"user_server_socket_path",
			"",
			"path to create socket for user server",
		)
		flRunnerServerUrl = flagset.String(
			"runner_server_url",
			"",
			"url of runner server",
		)
		flRunnerServerAuthToken = flagset.String(
			"runner_server_auth_token",
			"",
			"token used to auth with runner server",
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
		flIconPath = flagset.String(
			"icon_path",
			"",
			"path to icon file",
		)
		flDesktopEnabled = flagset.Bool(
			"desktop_enabled",
			false,
			"if desktop already enabled, show desktop immediately",
		)
	)

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	logLevel := slog.LevelInfo
	if *fldebug {
		logLevel = slog.LevelDebug
	}

	slogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	})).With(
		"subprocess", "desktop",
		"session_pid", os.Getpid(),
	)

	// GOMAXPROCS is set via environment variable by the desktop runner.
	// Go respects this natively, but we still apply gomaxprocsLimiter to get the logging.
	applyGomaxprocs(slogger)

	// Try to get the current user, so we can use the UID for logging. Not a fatal error if we can't, though
	user, err := user.Current()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelDebug,
			"error getting current user",
			"err", err,
		)
	} else {
		slogger = slogger.With("uid", user.Uid)
	}

	slogger.Log(context.TODO(), slog.LevelInfo,
		"starting",
	)

	if *flUserServerSocketPath == "" {
		*flUserServerSocketPath = defaultUserServerSocketPath()
		slogger.Log(context.TODO(), slog.LevelInfo,
			"using default socket path since none was provided",
			"socket_path", *flUserServerSocketPath,
		)
	}

	runGroup := rungroup.NewRunGroup()
	runGroup.SetSlogger(slogger)

	// listen for signals
	runGroup.Add("desktopSignalListener", func() error {
		listenSignals(slogger)
		return nil
	}, func(error) {})

	// monitor parent
	runGroup.Add("desktopMonitorParentProcess", func() error {
		monitorParentProcess(slogger, *flRunnerServerUrl, *flRunnerServerAuthToken, 2*time.Second)
		return nil
	}, func(error) {})

	shutdownChan := make(chan struct{})

	// if desktop is not enabled, we will wait for a signal to show it
	// on this channel
	var showDesktopChan chan struct{}
	if !*flDesktopEnabled {
		showDesktopChan = make(chan struct{})
	}

	// Set up notification sending and listening
	notifier := notify.NewDesktopNotifier(slogger, *flIconPath)
	runGroup.Add("desktopNotifier", notifier.Execute, notifier.Interrupt)

	server, err := userserver.New(slogger, *flUserServerAuthToken, *flUserServerSocketPath, shutdownChan, showDesktopChan, notifier)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"could not create user server",
			"err", err,
		)
		return fmt.Errorf("creating server: %w", err)
	}

	universalLinkHandler, urlInput := universallink.NewUniversalLinkHandler(slogger)
	runGroup.Add("universalLinkHandler", universalLinkHandler.Execute, universalLinkHandler.Interrupt)
	// Pass through channel so that systray can alert the link handler when it receives a universal link request
	m := menu.New(slogger, *flhostname, *flmenupath, urlInput)
	refreshMenu := func() {
		m.Build()
	}
	server.RegisterRefreshListener(refreshMenu)

	// start user server
	runGroup.Add("desktopServer", server.Serve, func(err error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slogger.Log(context.TODO(), slog.LevelError,
				"shutting down server",
				"err", err,
			)
		}
	})

	// listen on shutdown channel
	runGroup.Add("desktopServerShutdownListener", func() error {
		<-shutdownChan
		return nil
	}, func(err error) {
		m.Shutdown()
	})

	// notify runner server when menu opened
	runGroup.Add("desktopMenuOpenedListener", func() error {
		notifyRunnerServerMenuOpened(slogger, *flRunnerServerUrl, *flRunnerServerAuthToken)
		return nil
	}, func(err error) {})

	// run run group
	gowrapper.Go(context.TODO(), slogger, func() {
		// have to run this in a goroutine because menu needs the main thread
		if err := runGroup.Run(); err != nil {
			slogger.Log(context.TODO(), slog.LevelError,
				"running run group",
				"err", err,
			)
		}
	})

	// if desktop is not enabled at start up, wait for send on show desktop channel
	if !*flDesktopEnabled {
		<-showDesktopChan
	}

	// on darwin, if notifier.Listen() is called on a blocked main thread, it causes a crash,
	// so we wait until the main thread is unblocked to call it before initializing the menu.
	// this is noop for non-darwin
	notifier.Listen()
	// blocks until shutdown called
	m.Init()
	return nil
}

func listenSignals(slogger *slog.Logger) {
	signalsToHandle := []os.Signal{os.Interrupt, os.Kill}
	signals := make(chan os.Signal, len(signalsToHandle))
	signal.Notify(signals, signalsToHandle...)

	sig := <-signals

	slogger.Log(context.TODO(), slog.LevelDebug,
		"received signal",
		"signal", sig,
	)
}

type desktopClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func makeGetRequest(client desktopClient, requestUrl string, timeout time.Duration) (*http.Response, error) {
	if timeout == 0 {
		// Make sure we have a reasonable default value
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	return client.Do(req)
}

func notifyRunnerServerMenuOpened(slogger *slog.Logger, rootServerUrl, authToken string) {
	client := authedclient.New(authToken, 2*time.Second)
	menuOpenedUrl := fmt.Sprintf("%s%s", rootServerUrl, runnerserver.MenuOpenedEndpoint)

	for {
		<-systray.SystrayMenuOpened

		response, err := makeGetRequest(client, menuOpenedUrl, client.Timeout)
		if err != nil {
			slogger.Log(context.TODO(), slog.LevelError,
				"sending menu opened request to root server",
				"err", err,
			)
		}

		if response != nil {
			response.Body.Close()
		}
	}
}

// monitorParentProcess continuously checks to see if parent is a live and sends on provided channel if it is not
func monitorParentProcess(slogger *slog.Logger, runnerServerUrl, runnerServerAuthToken string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := authedclient.New(runnerServerAuthToken, interval)

	const maxErrCount = 3
	errCount := 0

	runnerHealthUrl := fmt.Sprintf("%s%s", runnerServerUrl, runnerserver.HealthCheckEndpoint)

	for ; true; <-ticker.C {
		// check to to ensure that the ppid is still legit
		if os.Getppid() < 2 {
			slogger.Log(context.TODO(), slog.LevelError,
				"ppid is 0 or 1, exiting",
			)
			break
		}

		response, err := makeGetRequest(client, runnerHealthUrl, client.Timeout)
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
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not connect to parent, will retry",
				"err", err,
				"attempts", errCount,
				"max_attempts", maxErrCount,
			)

			continue
		}

		// errCount >= maxErrCount, exit
		slogger.Log(context.TODO(), slog.LevelError,
			"could not connect to parent, max attempts reached, exiting",
			"err", err,
			"attempts", errCount,
			"max_attempts", maxErrCount,
		)

		break
	}
}

func defaultUserServerSocketPath() string {
	const socketBaseName = "kolide_desktop.sock"

	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\%s_%d_%s`, socketBaseName, os.Getpid(), ulid.New())
	}

	return agent.TempPath(fmt.Sprintf("%s_%d", socketBaseName, os.Getpid()))
}

// applyGomaxprocs reads the GOMAXPROCS environment variable and applies it via gomaxprocsLimiter
// to ensure proper logging. If GOMAXPROCS is not set, Go will use the number of CPUs by default.
func applyGomaxprocs(slogger *slog.Logger) {
	envValue := os.Getenv("GOMAXPROCS")
	if envValue == "" {
		// GOMAXPROCS not set, Go will use default (number of CPUs)
		return
	}

	parsedValue, err := strconv.Atoi(envValue)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"failed to parse GOMAXPROCS environment variable, using Go default",
			"env_value", envValue,
			"err", err,
		)
		return
	}

	gomaxprocsLimiter(context.TODO(), slogger, parsedValue)
}
