package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/cmd/launcher/internal/updater"
	"github.com/kolide/launcher/ee/control/consumers/keyvalueconsumer"
	"github.com/kolide/launcher/ee/control/consumers/notificationconsumer"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/localserver"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/storage"
	agentbbolt "github.com/kolide/launcher/pkg/agent/storage/bbolt"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/checkpoint"
	"github.com/kolide/launcher/pkg/osquery"
	osqueryInstanceHistory "github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/service"
	"github.com/oklog/run"

	"go.etcd.io/bbolt"
)

const (
	// Subsystems that launcher listens for control server updates on
	agentFlagsSubsystemName  = "agent_flags"
	serverDataSubsystemName  = "kolide_server_data"
	desktopMenuSubsystemName = "kolide_desktop_menu"
)

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), opts *launcher.Options) error {
	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

	// If delay_start is configured, wait before running launcher.
	if opts.DelayStart > 0*time.Second {
		level.Debug(logger).Log(
			"msg", "delay_start configured, waiting before starting launcher",
			"delay_start", opts.DelayStart.String(),
		)
		time.Sleep(opts.DelayStart)
	}

	level.Debug(logger).Log("msg", "runLauncher starting")

	// We've seen launcher intermittently be unable to recover from
	// DNS failures in the past, so this check gives us a little bit
	// of room to ensure that we are able to resolve DNS requests
	// before proceeding with starting launcher.
	if err := backoff.WaitFor(func() error {
		_, lookupErr := net.LookupIP(opts.KolideServerURL)
		return lookupErr
	}, 10*time.Second, 1*time.Second); err != nil {
		level.Info(logger).Log(
			"msg", "could not successfully perform IP lookup before starting launcher, proceeding anyway",
			"kolide_server_url", opts.KolideServerURL,
			"err", err,
		)
	}

	// determine the root directory, create one if it's not provided
	rootDirectory := opts.RootDirectory
	var err error
	if rootDirectory == "" {
		rootDirectory, err = agent.MkdirTemp(defaultRootDirectory)
		if err != nil {
			return fmt.Errorf("creating temporary root directory: %w", err)
		}
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if err := os.MkdirAll(rootDirectory, 0700); err != nil {
		return fmt.Errorf("creating root directory: %w", err)
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		return fmt.Errorf("detecting platform: %w", err)
	}

	debugAddrPath := filepath.Join(rootDirectory, "debug_addr")
	debug.AttachDebugHandler(debugAddrPath, logger)
	defer os.Remove(debugAddrPath)

	// open the database for storing launcher data, we do it here
	// because it's passed to multiple actors. Add a timeout to
	// this. Note that the timeout is documented as failing
	// unimplemented on windows, though empirically it seems to
	// work.
	boltOptions := &bbolt.Options{Timeout: time.Duration(30) * time.Second}
	db, err := bbolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, boltOptions)
	if err != nil {
		return fmt.Errorf("open launcher db: %w", err)
	}
	defer db.Close()

	if err := writePidFile(filepath.Join(rootDirectory, "launcher.pid")); err != nil {
		return fmt.Errorf("write launcher pid to file: %w", err)
	}

	stores, err := agentbbolt.MakeStores(logger, db)
	if err != nil {
		return fmt.Errorf("failed to create stores: %w", err)
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, stores[storage.AgentFlagsStore], fcOpts...)
	k := knapsack.New(stores, flagController, db)

	// construct the appropriate http client based on security settings
	httpClient := http.DefaultClient
	if k.InsecureTLS() {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	// If we have successfully opened the DB, and written a pid,
	// we expect we're live. Record the version for osquery to
	// pickup
	internal.RecordLauncherVersion(rootDirectory)

	// Try to ensure useful info in the logs
	checkpointer := checkpoint.New(logger, k)
	checkpointer.Run()

	// create the certificate pool
	var rootPool *x509.CertPool
	if k.RootPEM() != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := os.ReadFile(k.RootPEM())
		if err != nil {
			return fmt.Errorf("reading root certs PEM at path: %s: %w", k.RootPEM(), err)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return fmt.Errorf("found no valid certs in PEM at path: %s", k.RootPEM())
		}
	}
	// create a rungroup for all the actors we create to allow for easy start/stop
	var runGroup run.Group

	// Create a channel for signals
	sigChannel := make(chan os.Signal, 1)

	// Add a rungroup to catch things on the sigChannel
	// Attach a notifier for os.Interrupt
	runGroup.Add(func() error {
		signal.Notify(sigChannel, os.Interrupt)
		select {
		case <-sigChannel:
			level.Info(logger).Log("msg", "beginnning shutdown via signal")
			return nil
		}
	}, func(err error) {
		level.Info(logger).Log("msg", "interrupted", "err", err)
		level.Debug(logger).Log("msg", "interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))
		cancel()
		close(sigChannel)
	})

	var client service.KolideService
	{
		switch k.Transport() {
		case "grpc":
			grpcConn, err := service.DialGRPC(k.KolideServerURL(), k.InsecureTLS(), k.InsecureTransportTLS(), k.CertPins(), rootPool, logger)
			if err != nil {
				return fmt.Errorf("dialing grpc server: %w", err)
			}
			defer grpcConn.Close()
			client = service.NewGRPCClient(grpcConn, logger)
		case "jsonrpc":
			client = service.NewJSONRPCClient(k.KolideServerURL(), k.InsecureTLS(), k.InsecureTransportTLS(), k.CertPins(), rootPool, logger)
		case "osquery":
			client = service.NewNoopClient(logger)
		default:
			return errors.New("invalid transport option selected")
		}
	}

	// init osquery instance history
	if err := osqueryInstanceHistory.InitHistory(k.OsqueryHistoryInstanceStore()); err != nil {
		return fmt.Errorf("error initializing osquery instance history: %w", err)
	}

	// create the osquery extension for launcher. This is where osquery itself is launched.
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, k, client)
	if err != nil {
		return fmt.Errorf("create extension with runtime: %w", err)
	}
	runGroup.Add(extension.Execute, extension.Interrupt)

	versionInfo := version.Version()
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	go func() {
		// Sleep to give osquery time to startup before the checkpointer starts using it.
		time.Sleep(30 * time.Second)
		checkpointer.SetQuerier(extension)
	}()

	// Create the control service and services that depend on it
	var runner *desktopRunner.DesktopUsersProcessesRunner
	if k.ControlServerURL() == "" {
		level.Debug(logger).Log("msg", "control server URL not set, will not create control service")
	} else {
		controlService, err := createControlService(ctx, logger, k.ControlStore(), k)
		if err != nil {
			return fmt.Errorf("failed to setup control service: %w", err)
		}
		runGroup.Add(controlService.ExecuteWithContext(ctx), controlService.Interrupt)

		// serverDataConsumer handles server data table updates
		serverDataConsumer := keyvalueconsumer.New(k.ServerProvidedDataStore())
		controlService.RegisterConsumer(serverDataSubsystemName, serverDataConsumer)
		// agentFlagConsumer handles agent flags pushed from the control server
		agentFlagsConsumer := keyvalueconsumer.New(flagController)
		controlService.RegisterConsumer(agentFlagsSubsystemName, agentFlagsConsumer)

		runner, err = desktopRunner.New(
			k,
			desktopRunner.WithLogger(logger),
			desktopRunner.WithAuthToken(ulid.New()),
			desktopRunner.WithUsersFilesRoot(rootDirectory),
		)
		if err != nil {
			return fmt.Errorf("failed to create desktop runner: %w", err)
		}

		runGroup.Add(runner.Execute, runner.Interrupt)
		controlService.RegisterConsumer(desktopMenuSubsystemName, runner)
		// Run the notification service
		notificationConsumer, err := notificationconsumer.NewNotifyConsumer(
			k.SentNotificationsStore(),
			runner,
			ctx,
			notificationconsumer.WithLogger(logger),
		)
		if err != nil {
			return fmt.Errorf("failed to set up notifier: %w", err)
		}
		// Runs the cleanup routine for old notification records
		runGroup.Add(notificationConsumer.Execute, notificationConsumer.Interrupt)

		if err := controlService.RegisterConsumer(notificationconsumer.NotificationSubsystem, notificationConsumer); err != nil {
			return fmt.Errorf("failed to register notify consumer: %w", err)
		}
	}

	runEECode := k.ControlServerURL() != "" || k.IAmBreakingEELicense()

	// at this moment, these values are the same. This variable is here to help humans parse what's happening
	runLocalServer := runEECode
	if runLocalServer {
		ls, err := localserver.New(
			k,
			localserver.WithLogger(logger),
		)

		if err != nil {
			// For now, log this and move on. It might be a fatal error
			level.Error(logger).Log("msg", "Failed to setup localserver", "error", err)
		}

		ls.SetQuerier(extension)
		runGroup.Add(ls.Start, ls.Interrupt)
	}

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if k.Autoupdate() {
		osqueryUpdaterconfig := &updater.UpdaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: k.AutoupdateInterval(),
			UpdateChannel:      autoupdate.UpdateChannel(k.UpdateChannel()),
			NotaryURL:          k.NotaryServerURL(),
			MirrorURL:          k.MirrorServerURL(),
			NotaryPrefix:       k.NotaryPrefix(),
			HTTPClient:         httpClient,
			InitialDelay:       k.AutoupdateInitialDelay() + k.AutoupdateInterval()/2,
			SigChannel:         sigChannel,
		}

		// create an updater for osquery
		osqueryLegacyUpdater, err := updater.NewUpdater(ctx, opts.OsquerydPath, runnerRestart, osqueryUpdaterconfig)
		if err != nil {
			return fmt.Errorf("create osquery updater: %w", err)
		}
		runGroup.Add(osqueryLegacyUpdater.Execute, osqueryLegacyUpdater.Interrupt)

		launcherUpdaterconfig := &updater.UpdaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: k.AutoupdateInterval(),
			UpdateChannel:      autoupdate.UpdateChannel(k.UpdateChannel()),
			NotaryURL:          k.NotaryServerURL(),
			MirrorURL:          k.MirrorServerURL(),
			NotaryPrefix:       k.NotaryPrefix(),
			HTTPClient:         httpClient,
			InitialDelay:       k.AutoupdateInitialDelay(),
			SigChannel:         sigChannel,
		}

		// create an updater for launcher
		launcherPath, err := os.Executable()
		if err != nil {
			logutil.Fatal(logger, "err", err)
		}
		launcherLegacyUpdater, err := updater.NewUpdater(
			ctx,
			launcherPath,
			updater.UpdateFinalizer(logger, func() error {
				// stop desktop on auto updates
				if runner != nil {
					runner.Interrupt(nil)
				}
				return runnerShutdown()
			}),
			launcherUpdaterconfig,
		)
		if err != nil {
			return fmt.Errorf("create launcher updater: %w", err)
		}
		runGroup.Add(launcherLegacyUpdater.Execute, launcherLegacyUpdater.Interrupt)

		// Create a new TUF autoupdater
		metadataClient := http.DefaultClient
		metadataClient.Timeout = 1 * time.Minute
		mirrorClient := http.DefaultClient
		mirrorClient.Timeout = 8 * time.Minute // gives us extra time to avoid a timeout on download
		tufAutoupdater, err := tuf.NewTufAutoupdater(
			k,
			metadataClient,
			mirrorClient,
			extension,
			tuf.WithLogger(logger),
		)
		if err != nil {
			// Log the error, but don't return it -- the new TUF autoupdater is not critical yet
			level.Debug(logger).Log("msg", "could not create TUF autoupdater", "err", err)
		} else {
			runGroup.Add(tufAutoupdater.Execute, tufAutoupdater.Interrupt)
		}
	}

	if err := runGroup.Run(); err != nil {
		return fmt.Errorf("run service: %w", err)
	}

	return nil
}

func writePidFile(path string) error {
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return fmt.Errorf("writing pidfile: %w", err)
	}
	return nil
}
