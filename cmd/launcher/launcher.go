package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/cmd/launcher/internal/updater"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/storage"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/control/actionqueue"
	"github.com/kolide/launcher/ee/control/consumers/acceleratecontrolconsumer"
	"github.com/kolide/launcher/ee/control/consumers/flareconsumer"
	"github.com/kolide/launcher/ee/control/consumers/keyvalueconsumer"
	"github.com/kolide/launcher/ee/control/consumers/notificationconsumer"
	"github.com/kolide/launcher/ee/debug/checkups"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/localserver"
	"github.com/kolide/launcher/ee/powereventwatcher"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/logshipper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	osqueryInstanceHistory "github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/rungroup"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces/exporter"

	"go.etcd.io/bbolt"
)

const (
	// Subsystems that launcher listens for control server updates on
	agentFlagsSubsystemName  = "agent_flags"
	serverDataSubsystemName  = "kolide_server_data"
	desktopMenuSubsystemName = "kolide_desktop_menu"
	authTokensSubsystemName  = "auth_tokens"
)

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), slogger, systemSlogger *multislogger.MultiSlogger, opts *launcher.Options) error {
	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	logger := ctxlog.FromContext(ctx)
	logger = log.With(logger, "caller", log.DefaultCaller, "session_pid", os.Getpid())

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
	//
	// Note that the SplitN won't work for bare ip6 addresses.
	if err := backoff.WaitFor(func() error {
		hostport := strings.SplitN(opts.KolideServerURL, ":", 2)
		_, lookupErr := net.LookupIP(hostport[0])
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
		rootDirectory, err = agent.MkdirTemp(launcher.DefaultRootDirectoryPath)
		if err != nil {
			return fmt.Errorf("creating temporary root directory: %w", err)
		}
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if err := os.MkdirAll(rootDirectory, fsutil.DirMode); err != nil {
		return fmt.Errorf("creating root directory: %w", err)
	}
	// Ensure permissions are correct, regardless of umask settings -- we use
	// DirMode (0755) because the desktop processes that run as the user
	// must be able to access the root directory as well.
	if err := os.Chmod(rootDirectory, fsutil.DirMode); err != nil {
		return fmt.Errorf("chmodding root directory: %w", err)
	}
	if filepath.Dir(rootDirectory) == "/var/kolide-k2" {
		// We need to ensure the same for the parent of the root directory, but we only
		// want to do the same for Kolide-created directories.
		if err := os.Chmod(filepath.Dir(rootDirectory), fsutil.DirMode); err != nil {
			return fmt.Errorf("chmodding root directory parent: %w", err)
		}
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
	k := knapsack.New(stores, flagController, db, slogger, systemSlogger)

	go runOsqueryVersionCheck(ctx, logger, k.LatestOsquerydPath(ctx))

	if k.Debug() {
		// If we're in debug mode, then we assume we want to echo _all_ logs to stderr.
		k.AddSlogHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}))
	}

	// create a rungroup for all the actors we create to allow for easy start/stop
	runGroup := rungroup.NewRunGroup(logger)

	// Need to set up the log shipper so that we can get the logger early
	// and pass it to the various systems.
	var logShipper *logshipper.LogShipper
	var traceExporter *exporter.TraceExporter
	if k.ControlServerURL() != "" {

		initialDebugDuration := 10 * time.Minute

		// Set log shipping level to debug for the first X minutes of
		// run time. This will also increase the sending frequency.
		k.SetLogShippingLevelOverride("debug", initialDebugDuration)

		logShipper = logshipper.New(k, logger)
		runGroup.Add("logShipper", logShipper.Run, logShipper.Stop)

		logger = teelogger.New(logger, logShipper)
		logger = log.With(logger, "caller", log.Caller(5))
		k.AddSlogHandler(logShipper.SlogHandler())
		ctx = ctxlog.NewContext(ctx, logger) // Set the logger back in the ctx

		k.SetTraceSamplingRateOverride(1.0, initialDebugDuration)
		k.SetExportTracesOverride(true, initialDebugDuration)

		traceExporter, err = exporter.NewTraceExporter(ctx, k, logger)
		if err != nil {
			level.Debug(logger).Log(
				"msg", "could not set up trace exporter",
				"err", err,
			)
		} else {
			runGroup.Add("traceExporter", traceExporter.Execute, traceExporter.Interrupt)
		}
	}

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

	// Add the log checkpoints to the rungroup, and run it once early, to try to get data into the logs.
	checkpointer := checkups.NewCheckupLogger(logger, k)
	checkpointer.Once(ctx)
	runGroup.Add("logcheckpoint", checkpointer.Run, checkpointer.Interrupt)

	// Create a channel for signals
	sigChannel := make(chan os.Signal, 1)

	// Add a rungroup to catch things on the sigChannel
	signalListener := newSignalListener(sigChannel, cancel, logger)
	runGroup.Add("sigChannel", signalListener.Execute, signalListener.Interrupt)

	agent.ResetDatabaseIfNeeded(ctx, k)

	powerEventWatcher, err := powereventwatcher.New(k, log.With(logger, "component", "power_event_watcher"))
	if err != nil {
		level.Debug(logger).Log("msg", "could not init power event watcher", "err", err)
	} else {
		runGroup.Add("powerEventWatcher", powerEventWatcher.Execute, powerEventWatcher.Interrupt)
	}

	var client service.KolideService
	{
		switch k.Transport() {
		case "grpc":
			grpcConn, err := service.DialGRPC(k, rootPool)
			if err != nil {
				return fmt.Errorf("dialing grpc server: %w", err)
			}
			defer grpcConn.Close()
			client = service.NewGRPCClient(k, grpcConn)
		case "jsonrpc":
			client = service.NewJSONRPCClient(k, rootPool)
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
	runGroup.Add("osqueryExtension", extension.Execute, extension.Interrupt)

	versionInfo := version.Version()
	k.SystemSlogger().Info("started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	if traceExporter != nil {
		traceExporter.SetOsqueryClient(extension)
	}

	// Create the control service and services that depend on it
	var runner *desktopRunner.DesktopUsersProcessesRunner
	if k.ControlServerURL() == "" {
		level.Debug(logger).Log("msg", "control server URL not set, will not create control service")
	} else {
		controlService, err := createControlService(ctx, logger, k.ControlStore(), k)
		if err != nil {
			return fmt.Errorf("failed to setup control service: %w", err)
		}
		runGroup.Add("controlService", controlService.ExecuteWithContext(ctx), controlService.Interrupt)

		// serverDataConsumer handles server data table updates
		controlService.RegisterConsumer(serverDataSubsystemName, keyvalueconsumer.New(k.ServerProvidedDataStore()))
		// agentFlagConsumer handles agent flags pushed from the control server
		controlService.RegisterConsumer(agentFlagsSubsystemName, keyvalueconsumer.New(flagController))

		runner, err = desktopRunner.New(
			k,
			desktopRunner.WithLogger(logger),
			desktopRunner.WithAuthToken(ulid.New()),
			desktopRunner.WithUsersFilesRoot(rootDirectory),
		)
		if err != nil {
			return fmt.Errorf("failed to create desktop runner: %w", err)
		}

		runGroup.Add("desktopRunner", runner.Execute, runner.Interrupt)
		controlService.RegisterConsumer(desktopMenuSubsystemName, runner)

		// create an action queue for all other action style commands
		actionsQueue := actionqueue.New(
			actionqueue.WithContext(ctx),
			actionqueue.WithLogger(logger),
			actionqueue.WithStore(k.ControlServerActionsStore()),
			actionqueue.WithOldNotificationsStore(k.SentNotificationsStore()),
		)
		runGroup.Add("actionsQueue", actionsQueue.StartCleanup, actionsQueue.StopCleanup)
		controlService.RegisterConsumer(actionqueue.ActionsSubsystem, actionsQueue)

		// register accelerate control consumer
		actionsQueue.RegisterActor(acceleratecontrolconsumer.AccelerateControlSubsystem, acceleratecontrolconsumer.New(k))

		// register flare consumer
		actionsQueue.RegisterActor(flareconsumer.FlareSubsystem, flareconsumer.New(k))

		// create notification consumer
		notificationConsumer, err := notificationconsumer.NewNotifyConsumer(
			runner,
			ctx,
			notificationconsumer.WithLogger(logger),
		)
		if err != nil {
			return fmt.Errorf("failed to set up notifier: %w", err)
		}

		// register notifications consumer
		actionsQueue.RegisterActor(notificationconsumer.NotificationSubsystem, notificationConsumer)

		// Set up our tracing instrumentation
		authTokenConsumer := keyvalueconsumer.New(k.TokenStore())
		if err := controlService.RegisterConsumer(authTokensSubsystemName, authTokenConsumer); err != nil {
			return fmt.Errorf("failed to register auth token consumer: %w", err)
		}

		// begin log shipping and subsribe to token updates
		// nil check incase it failed to create for some reason
		if logShipper != nil {
			controlService.RegisterSubscriber(authTokensSubsystemName, logShipper)
		}

		if traceExporter != nil {
			controlService.RegisterSubscriber(authTokensSubsystemName, traceExporter)
		}

		if metadataWriter := internal.NewMetadataWriter(logger, k); metadataWriter == nil {
			level.Debug(logger).Log(
				"msg", "unable to set up metadata writer",
				"err", err,
			)
		} else {
			controlService.RegisterSubscriber(serverDataSubsystemName, metadataWriter)
			// explicitly trigger the ping at least once to ensure updated metadata is written
			// on upgrades, the subscriber will continue to do this automatically when new
			// information is made available from server_data (e.g. on a fresh install)
			metadataWriter.Ping()
		}
	}

	runEECode := k.ControlServerURL() != "" || k.IAmBreakingEELicense()

	// at this moment, these values are the same. This variable is here to help humans parse what's happening
	runLocalServer := runEECode
	if runLocalServer {
		ls, err := localserver.New(
			k,
		)

		if err != nil {
			// For now, log this and move on. It might be a fatal error
			level.Error(logger).Log("msg", "Failed to setup localserver", "error", err)
		}

		ls.SetQuerier(extension)
		runGroup.Add("localserver", ls.Start, ls.Interrupt)
	}

	// If autoupdating is enabled, run the new autoupdater
	if k.Autoupdate() {
		// Create a new TUF autoupdater
		metadataClient := http.DefaultClient
		metadataClient.Timeout = 30 * time.Second
		mirrorClient := http.DefaultClient
		mirrorClient.Timeout = 8 * time.Minute // gives us extra time to avoid a timeout on download
		tufAutoupdater, err := tuf.NewTufAutoupdater(
			k,
			metadataClient,
			mirrorClient,
			extension,
			tuf.WithLogger(logger),
			tuf.WithOsqueryRestart(runnerRestart),
		)
		if err != nil {
			return fmt.Errorf("creating TUF autoupdater updater: %w", err)
		}

		runGroup.Add("tufAutoupdater", tufAutoupdater.Execute, tufAutoupdater.Interrupt)
	}

	// Run the legacy autoupdater only if autoupdating is enabled and the given channel hasn't moved
	// to the new autoupdater yet.
	if k.Autoupdate() && !tuf.ChannelUsesNewAutoupdater(k.UpdateChannel()) {
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
		runGroup.Add("osqueryLegacyAutoupdater", osqueryLegacyUpdater.Execute, osqueryLegacyUpdater.Interrupt)

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
		runGroup.Add("launcherLegacyAutoupdater", launcherLegacyUpdater.Execute, launcherLegacyUpdater.Interrupt)
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

// runOsqueryVersionCheck execs the osqueryd binary in the background when we're running
// on darwin. Operating on our theory that some startup delay issues for osquery might
// be due to the notarization check taking too long, we execute the binary here ahead
// of time in the hopes of getting the check out of the way. This is expected to be called
// from a goroutine, and thus does not return an error.
func runOsqueryVersionCheck(ctx context.Context, logger log.Logger, osquerydPath string) {
	if runtime.GOOS != "darwin" {
		return
	}

	logger = log.With(logger, "component", "osquery-version-check")

	var output bytes.Buffer

	osq, err := runsimple.NewOsqueryProcess(osquerydPath, runsimple.WithStdout(&output))
	if err != nil {
		level.Error(logger).Log("msg", "unable to create process", "err", err)
		return
	}

	// This has a somewhat long timeout, in case there's a notarization fetch
	versionCtx, versionCancel := context.WithTimeout(ctx, 30*time.Second)
	defer versionCancel()

	startTime := time.Now().UnixMilli()

	osqErr := osq.RunVersion(versionCtx)
	executionTimeMs := time.Now().UnixMilli() - startTime
	outTrimmed := strings.TrimSpace(output.String())

	if osqErr != nil {
		level.Error(logger).Log("msg", "could not check osqueryd version",
			"output", outTrimmed,
			"err", err,
			"execution_time_ms", executionTimeMs,
			"osqueryd_path", osquerydPath,
		)
		return
	}

	level.Debug(logger).Log("msg", "checked osqueryd version",
		"version", outTrimmed,
		"execution_time_ms", executionTimeMs,
		"osqueryd_path", osquerydPath,
	)
}
