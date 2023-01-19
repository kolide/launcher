package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/cmd/launcher/internal/updater"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/ee/control/consumers/notificationconsumer"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/localserver"
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

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), opts *launcher.Options) error {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)
	level.Debug(logger).Log("msg", "runLauncher starting")

	// determine the root directory, create one if it's not provided
	rootDirectory := opts.RootDirectory
	if rootDirectory == "" {
		rootDirectory = filepath.Join(os.TempDir(), defaultRootDirectory)
		if _, err := os.Stat(rootDirectory); os.IsNotExist(err) {
			if err := os.Mkdir(rootDirectory, fsutil.DirMode); err != nil {
				return fmt.Errorf("creating temporary root directory: %w", err)
			}
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

	// construct the appropriate http client based on security settings
	httpClient := http.DefaultClient
	if opts.InsecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

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

	// If we have successfully opened the DB, and written a pid,
	// we expect we're live. Record the version for osquery to
	// pickup
	internal.RecordLauncherVersion(rootDirectory)

	// Try to ensure useful info in the logs
	checkpoint.Run(logger, db, *opts)

	// create the certificate pool
	var rootPool *x509.CertPool
	if opts.RootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := os.ReadFile(opts.RootPEM)
		if err != nil {
			return fmt.Errorf("reading root certs PEM at path: %s: %w", opts.RootPEM, err)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return fmt.Errorf("found no valid certs in PEM at path: %s", opts.RootPEM)
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
		switch opts.Transport {
		case "grpc":
			grpcConn, err := service.DialGRPC(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootPool, logger)
			if err != nil {
				return fmt.Errorf("dialing grpc server: %w", err)
			}
			defer grpcConn.Close()
			client = service.NewGRPCClient(grpcConn, logger)
		case "jsonrpc":
			client = service.NewJSONRPCClient(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootPool, logger)
		case "osquery":
			client = service.NewNoopClient(logger)
		default:
			return errors.New("invalid transport option selected")
		}
	}

	// init osquery instance history
	if err := osqueryInstanceHistory.InitHistory(db); err != nil {
		return fmt.Errorf("error initializing osquery instance history: %w", err)
	}

	// create the osquery extension for launcher. This is where osquery itself is launched.
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, db, client, opts)
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

	controlService, err := createControlService(ctx, logger, opts)
	if err != nil {
		return fmt.Errorf("failed to setup control service: %w", err)
	}
	runGroup.Add(controlService.Execute, controlService.Interrupt)

	// serverDataBucketConsumer handles server data table updates
	serverDataBucketConsumer := control.NewBucketConsumer(logger, db, osquery.ServerProvidedDataBucket)
	controlService.RegisterConsumer("kolide_server_data", serverDataBucketConsumer)

	desktopFlagsBucketConsumer := control.NewBucketConsumer(logger, db, "agent_flags")
	controlService.RegisterConsumer("agent_flags", desktopFlagsBucketConsumer)

	runner := desktopRunner.New(
		desktopRunner.WithLogger(logger),
		desktopRunner.WithUpdateInterval(time.Second*5),
		desktopRunner.WithHostname(opts.KolideServerURL),
		desktopRunner.WithAuthToken(ulid.New()),
		desktopRunner.WithUsersFilesRoot(rootDirectory),
		desktopRunner.WithProcessSpawningEnabled(opts.KolideServerURL == "k2device-preprod.kolide.com" || opts.KolideServerURL == "localhost:3443"),
		desktopRunner.WithStoredDataProvider(desktopFlagsBucketConsumer),
	)
	runGroup.Add(runner.Execute, runner.Interrupt)
	controlService.RegisterConsumer("kolide_desktop_menu", runner)
	controlService.RegisterSubscriber("kolide_desktop_flags", runner)

	// Run the notification service
	notificationConsumer, err := notificationconsumer.NewNotifyConsumer(
		db,
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

	if opts.KolideServerURL == "k2device.kolide.com" ||
		opts.KolideServerURL == "k2device-preprod.kolide.com" ||
		opts.KolideServerURL == "localhost:3443" ||
		strings.HasSuffix(opts.KolideServerURL, "herokuapp.com") {

		ls, err := localserver.New(logger, db, opts.KolideServerURL)
		if err != nil {
			// For now, log this and move on. It might be a fatal error
			level.Error(logger).Log("msg", "Failed to setup localserver", "error", err)
		}

		ls.SetQuerier(extension)
		runGroup.Add(ls.Start, ls.Interrupt)
	}

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if opts.Autoupdate {
		osqueryUpdaterconfig := &updater.UpdaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.AutoupdateInterval,
			UpdateChannel:      opts.UpdateChannel,
			NotaryURL:          opts.NotaryServerURL,
			MirrorURL:          opts.MirrorServerURL,
			NotaryPrefix:       opts.NotaryPrefix,
			HTTPClient:         httpClient,
			InitialDelay:       opts.AutoupdateInitialDelay + opts.AutoupdateInterval/2,
			SigChannel:         sigChannel,
		}

		// create an updater for osquery
		osqueryUpdater, err := updater.NewUpdater(ctx, opts.OsquerydPath, runnerRestart, osqueryUpdaterconfig)
		if err != nil {
			return fmt.Errorf("create osquery updater: %w", err)
		}
		runGroup.Add(osqueryUpdater.Execute, osqueryUpdater.Interrupt)

		launcherUpdaterconfig := &updater.UpdaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.AutoupdateInterval,
			UpdateChannel:      opts.UpdateChannel,
			NotaryURL:          opts.NotaryServerURL,
			MirrorURL:          opts.MirrorServerURL,
			NotaryPrefix:       opts.NotaryPrefix,
			HTTPClient:         httpClient,
			InitialDelay:       opts.AutoupdateInitialDelay,
			SigChannel:         sigChannel,
		}

		// create an updater for launcher
		launcherPath, err := os.Executable()
		if err != nil {
			logutil.Fatal(logger, "err", err)
		}
		launcherUpdater, err := updater.NewUpdater(
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
		runGroup.Add(launcherUpdater.Execute, launcherUpdater.Interrupt)
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
