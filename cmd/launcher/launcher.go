package main

import (
	"bytes"
	"context"
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
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/ee/agent/storage"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/timemachine"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/ee/control/actionqueue"
	"github.com/kolide/launcher/ee/control/consumers/acceleratecontrolconsumer"
	"github.com/kolide/launcher/ee/control/consumers/flareconsumer"
	"github.com/kolide/launcher/ee/control/consumers/keyvalueconsumer"
	"github.com/kolide/launcher/ee/control/consumers/notificationconsumer"
	"github.com/kolide/launcher/ee/control/consumers/remoterestartconsumer"
	"github.com/kolide/launcher/ee/control/consumers/uninstallconsumer"
	"github.com/kolide/launcher/ee/debug/checkups"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/localserver"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/observability/exporter"
	"github.com/kolide/launcher/ee/powereventwatcher"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/ee/watchdog"
	"github.com/kolide/launcher/pkg/augeas"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/logshipper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	osqueryruntime "github.com/kolide/launcher/pkg/osquery/runtime"
	osqueryInstanceHistory "github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/rungroup"
	"github.com/kolide/launcher/pkg/service"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"go.etcd.io/bbolt"
)

const (
	// Subsystems that launcher listens for control server updates on
	agentFlagsSubsystemName  = "agent_flags"
	serverDataSubsystemName  = "kolide_server_data"
	desktopMenuSubsystemName = "kolide_desktop_menu"
	authTokensSubsystemName  = "auth_tokens"
	katcSubsystemName        = "katc_config" // Kolide ATC
	ztaInfoSubsystemName     = "zta_info"    // legacy name for dt4aInfo subsystem
	dt4aInfoSubsystemName    = "dt4a_info"
)

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), multiSlogger, systemMultiSlogger *multislogger.MultiSlogger, opts *launcher.Options) error {
	initialTraceBuffer := exporter.NewInitialTraceBuffer()
	ctx, startupSpan := observability.StartSpan(ctx)

	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	logger := ctxlog.FromContext(ctx)
	logger = log.With(logger, "caller", log.DefaultCaller, "session_pid", os.Getpid())
	slogger := multiSlogger.Logger

	// If delay_start is configured, wait before running launcher.
	if opts.DelayStart > 0*time.Second {
		slogger.Log(ctx, slog.LevelDebug,
			"delay_start configured, waiting before starting launcher",
			"delay_start", opts.DelayStart.String(),
		)
		time.Sleep(opts.DelayStart)
		startupSpan.AddEvent("delay_start_completed")
	}

	slogger.Log(ctx, slog.LevelDebug,
		"runLauncher starting",
	)

	// We've seen launcher intermittently be unable to recover from
	// DNS failures in the past, so this check gives us a little bit
	// of room to ensure that we are able to resolve DNS requests
	// before proceeding with starting launcher.
	//
	// Note that the SplitN won't work for bare ip6 addresses.
	if err := backoff.WaitFor(func() error {
		hostport := strings.SplitN(opts.KolideServerURL, ":", 2)
		if len(hostport) < 1 {
			return fmt.Errorf("unable to parse url: %s", opts.KolideServerURL)
		}
		_, lookupErr := net.LookupIP(hostport[0])
		return lookupErr
	}, 10*time.Second, 1*time.Second); err != nil {
		slogger.Log(ctx, slog.LevelInfo,
			"could not successfully perform IP lookup before starting launcher, proceeding anyway",
			"kolide_server_url", opts.KolideServerURL,
			"err", err,
		)
	}
	startupSpan.AddEvent("dns_lookup_completed")

	// determine the root directory, create one if it's not provided
	rootDirectory := opts.RootDirectory
	var err error
	if rootDirectory == "" {
		rootDirectory, err = agent.MkdirTemp(launcher.DefaultRootDirectoryPath)
		if err != nil {
			return fmt.Errorf("creating temporary root directory: %w", err)
		}

		slogger.Log(ctx, slog.LevelInfo,
			"using default system root directory",
			"path", rootDirectory,
		)
		// Make sure we have record of this new root directory in the opts, so it will be set
		// correctly in the knapsack later.
		opts.RootDirectory = rootDirectory
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
	startupSpan.AddEvent("root_directory_created")

	if _, err := osquery.DetectPlatform(); err != nil {
		return fmt.Errorf("detecting platform: %w", err)
	}

	debugAddrPath := filepath.Join(rootDirectory, "debug_addr")
	debug.AttachDebugHandler(debugAddrPath, slogger)
	defer os.Remove(debugAddrPath)

	// open the database for storing launcher data, we do it here
	// because it's passed to multiple actors. Add a timeout to
	// this. Note that the timeout is documented as failing
	// unimplemented on windows, though empirically it seems to
	// work.
	agentbbolt.UseBackupDbIfNeeded(rootDirectory, slogger)
	boltOptions := &bbolt.Options{
		Timeout:      time.Duration(30) * time.Second,
		FreelistType: bbolt.FreelistMapType,
	}
	db, err := bbolt.Open(agentbbolt.LauncherDbLocation(rootDirectory), 0600, boltOptions)
	if err != nil {
		return fmt.Errorf("open launcher db: %w", err)
	}
	defer db.Close()
	startupSpan.AddEvent("database_opened")

	if err := writePidFile(filepath.Join(rootDirectory, "launcher.pid")); err != nil {
		return fmt.Errorf("write launcher pid to file: %w", err)
	}

	stores, err := agentbbolt.MakeStores(ctx, slogger, db)
	if err != nil {
		return fmt.Errorf("failed to create stores: %w", err)
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(slogger, stores[storage.AgentFlagsStore], fcOpts...)
	k := knapsack.New(stores, flagController, db, multiSlogger, systemMultiSlogger)

	// Generate a new run ID
	newRunID := k.GetRunID()

	// Apply the run ID to both logger and slogger
	logger = log.With(logger, "run_id", newRunID)
	slogger = slogger.With("run_id", newRunID)

	// set start time, first runtime, first version
	initLauncherHistory(k)

	gowrapper.Go(ctx, slogger, func() {
		osquery.CollectAndSetEnrollmentDetails(ctx, slogger, k, 60*time.Second, 6*time.Second)
	})
	gowrapper.Go(ctx, slogger, func() {
		runOsqueryVersionCheckAndAddToKnapsack(ctx, slogger, k, k.LatestOsquerydPath(ctx))
	})
	gowrapper.Go(ctx, slogger, func() {
		// Wait a little bit before adding exclusions -- some osquery files get created right after
		// startup and we want to let that settle before handling exclusions.
		time.Sleep(2 * time.Minute)
		timemachine.AddExclusions(ctx, k)
	})

	if k.Debug() && runtime.GOOS != "windows" {
		// If we're in debug mode, then we assume we want to echo _all_ logs to stderr.
		k.AddSlogHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}))
	}

	// create a rungroup for all the actors we create to allow for easy start/stop
	runGroup := rungroup.NewRunGroup()

	// Need to set up the log shipper so that we can get the logger early
	// and pass it to the various systems.
	var logShipper *logshipper.LogShipper
	var telemetryExporter *exporter.TelemetryExporter
	if k.ControlServerURL() != "" {
		startupSpan.AddEvent("log_shipper_init_start")

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

		telemetryExporter, err = exporter.NewTelemetryExporter(ctx, k, initialTraceBuffer)
		if err != nil {
			slogger.Log(ctx, slog.LevelDebug,
				"could not set up telemetry exporter",
				"err", err,
			)
		} else {
			runGroup.Add("telemetryExporter", telemetryExporter.Execute, telemetryExporter.Interrupt)
		}

		startupSpan.AddEvent("log_shipper_init_completed")
	}

	// Now that log shipping is set up, set the slogger on the rungroup so that rungroup logs
	// will also be shipped.
	runGroup.SetSlogger(k.Slogger())

	startupSettingsWriter, err := startupsettings.OpenWriter(ctx, k)
	if err != nil {
		return fmt.Errorf("creating startup db: %w", err)
	}
	defer startupSettingsWriter.Close()

	if err := startupSettingsWriter.WriteSettings(); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"writing startup settings",
			"err", err,
		)
	}

	// If we have successfully opened the DB, and written a pid,
	// we expect we're live. Record the version for osquery to
	// pickup
	internal.RecordLauncherVersion(ctx, rootDirectory)

	dbBackupSaver := agentbbolt.NewDatabaseBackupSaver(k)
	runGroup.Add("dbBackupSaver", dbBackupSaver.Execute, dbBackupSaver.Interrupt)

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
	// The checkpointer can take up to 5 seconds to run, so do this in the background.
	checkpointer := checkups.NewCheckupLogger(slogger, k)
	gowrapper.Go(ctx, slogger, func() {
		checkpointer.Once(ctx)
	})
	runGroup.Add("logcheckpoint", checkpointer.Run, checkpointer.Interrupt)

	watchdogController, err := watchdog.NewController(ctx, k, opts.ConfigFilePath)
	if err != nil { // log any issues here but move on, watchdog is not critical path
		slogger.Log(ctx, slog.LevelError,
			"could not init watchdog controller",
			"err", err,
		)
	} else if watchdogController != nil { // watchdogController will be nil on non-windows platforms for now
		k.RegisterChangeObserver(watchdogController, keys.LauncherWatchdogEnabled)
		runGroup.Add("watchdogController", watchdogController.Run, watchdogController.Interrupt)
	}

	// Create a channel for signals
	sigChannel := make(chan os.Signal, 1)

	// Add a rungroup to catch things on the sigChannel
	signalListener := newSignalListener(sigChannel, cancel, slogger)
	runGroup.Add("sigChannel", signalListener.Execute, signalListener.Interrupt)

	// For now, remediation is not performed -- we only log the hardware change. So we can
	// perform this operation in the background to avoid slowing down launcher startup.
	gowrapper.Go(ctx, slogger, func() {
		agent.DetectAndRemediateHardwareChange(ctx, k)
	})

	powerEventSubscriber := powereventwatcher.NewKnapsackSleepStateUpdater(slogger, k)
	powerEventWatcher, err := powereventwatcher.New(ctx, slogger, powerEventSubscriber)
	if err != nil {
		slogger.Log(ctx, slog.LevelDebug,
			"could not init power event watcher",
			"err", err,
		)
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

	// make sure keys exist -- we expect these keys to exist before rungroup starts
	if err := agent.SetupKeys(ctx, k.Slogger(), k.ConfigStore()); err != nil {
		return fmt.Errorf("setting up agent keys: %w", err)
	}

	// init osquery instance history
	if osqHistory, err := osqueryInstanceHistory.InitHistory(k.OsqueryHistoryInstanceStore()); err != nil {
		return fmt.Errorf("error initializing osquery instance history: %w", err)
	} else {
		k.SetOsqueryHistory(osqHistory)
	}

	// create the runner that will launch osquery
	osqueryRunner := osqueryruntime.New(
		k,
		client,
		startupSettingsWriter,
		osqueryruntime.WithAugeasLensFunction(augeas.InstallLenses),
	)
	runGroup.Add("osqueryRunner", osqueryRunner.Run, osqueryRunner.Interrupt)
	k.SetInstanceQuerier(osqueryRunner)

	versionInfo := version.Version()
	k.SystemSlogger().Log(ctx, slog.LevelInfo,
		"started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	// Create the control service and services that depend on it
	var runner *desktopRunner.DesktopUsersProcessesRunner
	var actionsQueue *actionqueue.ActionQueue
	if k.ControlServerURL() == "" {
		slogger.Log(ctx, slog.LevelDebug,
			"control server URL not set, will not create control service",
		)
	} else {
		controlService, err := createControlService(ctx, k)
		if err != nil {
			return fmt.Errorf("failed to setup control service: %w", err)
		}
		runGroup.Add("controlService", controlService.ExecuteWithContext(ctx), controlService.Interrupt)

		// serverDataConsumer handles server data table updates
		controlService.RegisterConsumer(serverDataSubsystemName, keyvalueconsumer.New(k.ServerProvidedDataStore()))
		// agentFlagConsumer handles agent flags pushed from the control server
		controlService.RegisterConsumer(agentFlagsSubsystemName, keyvalueconsumer.New(flagController))
		// katcConfigConsumer handles updates to Kolide's custom ATC tables
		controlService.RegisterConsumer(katcSubsystemName, keyvalueconsumer.NewConfigConsumer(k.KatcConfigStore()))
		controlService.RegisterSubscriber(katcSubsystemName, osqueryRunner)
		controlService.RegisterSubscriber(katcSubsystemName, startupSettingsWriter)

		runner, err = desktopRunner.New(
			k,
			controlService,
			desktopRunner.WithAuthToken(ulid.New()),
			desktopRunner.WithUsersFilesRoot(rootDirectory),
		)
		if err != nil {
			return fmt.Errorf("failed to create desktop runner: %w", err)
		}

		execute, interrupt, err := agent.SetHardwareKeysRunner(ctx, k.Slogger(), k.ConfigStore(), runner)
		if err != nil {
			return fmt.Errorf("setting up hardware keys: %w", err)
		}
		runGroup.Add("hardwareKeys", execute, interrupt)

		runGroup.Add("desktopRunner", runner.Execute, runner.Interrupt)
		controlService.RegisterConsumer(desktopMenuSubsystemName, runner)

		// create an action queue for all other action style commands
		actionsQueue = actionqueue.New(
			k,
			actionqueue.WithContext(ctx),
			actionqueue.WithStore(k.ControlServerActionsStore()),
			actionqueue.WithOldNotificationsStore(k.SentNotificationsStore()),
		)
		runGroup.Add("actionsQueue", actionsQueue.StartCleanup, actionsQueue.StopCleanup)
		controlService.RegisterConsumer(actionqueue.ActionsSubsystem, actionsQueue)

		// register accelerate control consumer
		actionsQueue.RegisterActor(acceleratecontrolconsumer.AccelerateControlSubsystem, acceleratecontrolconsumer.New(k))
		// register uninstall consumer
		actionsQueue.RegisterActor(uninstallconsumer.UninstallSubsystem, uninstallconsumer.New(k))
		// register flare consumer
		actionsQueue.RegisterActor(flareconsumer.FlareSubsystem, flareconsumer.New(k))
		// register force full control data fetch consumer
		actionsQueue.RegisterActor(control.ForceFullControlDataFetchAction, controlService)

		// create notification consumer
		notificationConsumer, err := notificationconsumer.NewNotifyConsumer(
			ctx,
			k,
			runner,
		)
		if err != nil {
			return fmt.Errorf("failed to set up notifier: %w", err)
		}

		// register notifications consumer
		actionsQueue.RegisterActor(notificationconsumer.NotificationSubsystem, notificationConsumer)

		remoteRestartConsumer := remoterestartconsumer.New(k)
		runGroup.Add("remoteRestart", remoteRestartConsumer.Execute, remoteRestartConsumer.Interrupt)
		actionsQueue.RegisterActor(remoterestartconsumer.RemoteRestartActorType, remoteRestartConsumer)

		// Set up our tracing instrumentation
		authTokenConsumer := keyvalueconsumer.New(k.TokenStore())
		if err := controlService.RegisterConsumer(authTokensSubsystemName, authTokenConsumer); err != nil {
			return fmt.Errorf("failed to register auth token consumer: %w", err)
		}

		// begin log shipping and subscribe to token updates
		// nil check in case it failed to create for some reason
		if logShipper != nil {
			controlService.RegisterSubscriber(authTokensSubsystemName, logShipper)
		}

		if telemetryExporter != nil {
			controlService.RegisterSubscriber(authTokensSubsystemName, telemetryExporter)
		}

		if metadataWriter := internal.NewMetadataWriter(slogger, k); metadataWriter == nil {
			slogger.Log(ctx, slog.LevelDebug,
				"unable to set up metadata writer",
				"err", err,
			)
		} else {
			controlService.RegisterSubscriber(serverDataSubsystemName, metadataWriter)
			// explicitly trigger the ping at least once to ensure updated metadata is written
			// on upgrades, the subscriber will continue to do this automatically when new
			// information is made available from server_data (e.g. on a fresh install)
			metadataWriter.Ping()
		}

		// Set up consumer to receive DT4A info from the control server in both current and legacy subsystem
		dt4aInfoConsumer := keyvalueconsumer.NewConfigConsumer(k.Dt4aInfoStore())
		if err := controlService.RegisterConsumer(dt4aInfoSubsystemName, dt4aInfoConsumer); err != nil {
			return fmt.Errorf("failed to register dt4a info consumer: %w", err)
		}

		//ztaInfoConsumer is the legacy consumer for zta
		if err := controlService.RegisterConsumer(ztaInfoSubsystemName, dt4aInfoConsumer); err != nil {
			return fmt.Errorf("failed to register dt4a info consumer: %w", err)
		}
	}

	runEECode := k.ControlServerURL() != "" || k.IAmBreakingEELicense()

	// at this moment, these values are the same. This variable is here to help humans parse what's happening
	runLocalServer := runEECode
	if runLocalServer {
		ls, err := localserver.New(
			ctx,
			k,
			runner,
		)

		if err != nil {
			// For now, log this and move on. It might be a fatal error
			slogger.Log(ctx, slog.LevelError,
				"failed to setup local server",
				"err", err,
			)
		}

		ls.SetQuerier(osqueryRunner)
		runGroup.Add("localserver", ls.Start, ls.Interrupt)
	}

	// If autoupdating is enabled, run the autoupdater
	if k.Autoupdate() {
		metadataClient := http.DefaultClient
		metadataClient.Timeout = 30 * time.Second
		metadataClient.Transport = otelhttp.NewTransport(metadataClient.Transport)
		mirrorClient := http.DefaultClient
		mirrorClient.Timeout = 8 * time.Minute // gives us extra time to avoid a timeout on download
		mirrorClient.Transport = otelhttp.NewTransport(mirrorClient.Transport)
		tufAutoupdater, err := tuf.NewTufAutoupdater(
			ctx,
			k,
			metadataClient,
			mirrorClient,
			osqueryRunner,
			tuf.WithOsqueryRestart(osqueryRunner.Restart),
		)
		if err != nil {
			return fmt.Errorf("creating TUF autoupdater updater: %w", err)
		}

		runGroup.Add("tufAutoupdater", tufAutoupdater.Execute, tufAutoupdater.Interrupt)
		if actionsQueue != nil {
			actionsQueue.RegisterActor(tuf.AutoupdateSubsystemName, tufAutoupdater)
		}

		// in some cases, (e.g. rolling back a windows installation to a previous osquery version) it is possible that
		// the installer leaves us in a situation where there is no osqueryd on disk.
		// we can detect this and attempt to download the correct version into the TUF update library to run from that.
		// This must be done as a blocking operation before the rungroups start, because the osquery runner will fail to
		// launch and trigger a restart immediately
		currentOsquerydBinaryPath := k.LatestOsquerydPath(ctx)
		if _, err = os.Stat(currentOsquerydBinaryPath); os.IsNotExist(err) {
			slogger.Log(ctx, slog.LevelInfo,
				"detected missing osqueryd executable, will attempt to download",
			)

			startupSpan.AddEvent("osqueryd_startup_download_start")
			// simulate control server request for immediate update, noting to bypass the initial delay window
			actionReader := strings.NewReader(`{
				"bypass_initial_delay": true,
				"binaries_to_update": [
					{ "name": "osqueryd" }
				]
			}`)

			if err = tufAutoupdater.Do(actionReader); err != nil {
				slogger.Log(ctx, slog.LevelError,
					"failure triggering immediate osquery update",
					"err", err,
				)
			}

			startupSpan.AddEvent("osqueryd_startup_download_completed")
		}
	}

	startupSpan.End()

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

func initLauncherHistory(k types.Knapsack) {
	// start counting uptime (want to reset on every run)
	utcTimeNow := time.Now().UTC()
	if err := k.LauncherHistoryStore().Set([]byte("process_start_time"), []byte(utcTimeNow.Format(time.RFC3339))); err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"error setting process start time",
			"err", err,
		)
	}

	// set first recorded version (do not want to reset on every run)
	installVersion, err := k.LauncherHistoryStore().Get([]byte("first_recorded_version"))
	if err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"error getting first recorded version",
			"err", err,
		)

		installVersion = nil
	}

	if installVersion == nil {
		if err := k.LauncherHistoryStore().Set([]byte("first_recorded_version"), []byte(version.Version().Version)); err != nil {
			k.Slogger().Log(context.Background(), slog.LevelError,
				"error setting first recorded version",
				"err", err,
				"version", string(installVersion),
			)
		}
	}

	// set first recorded run time (do not want to reset on every run)
	installDateTime, err := k.LauncherHistoryStore().Get([]byte("first_recorded_run_time"))
	if err != nil {
		k.Slogger().Log(context.Background(), slog.LevelError,
			"error getting first recorded run time",
			"err", err,
		)

		installDateTime = nil
	}

	if installDateTime == nil {
		if err := k.LauncherHistoryStore().Set([]byte("first_recorded_run_time"), []byte(utcTimeNow.Format(time.RFC3339))); err != nil {
			k.Slogger().Log(context.Background(), slog.LevelError,
				"error setting first recorded run time",
				"err", err,
			)
		}
	}
}

// runOsqueryVersionCheckAndAddToKnapsack execs the osqueryd binary in the background when we're running
// on to check the version and save it in the Knapsack. This is expected to be called
// from a goroutine, and thus does not return an error.
func runOsqueryVersionCheckAndAddToKnapsack(ctx context.Context, slogger *slog.Logger, k types.Knapsack, osquerydPath string) {

	slogger = slogger.With("component", "osquery-version-check")

	var output bytes.Buffer

	osq, err := runsimple.NewOsqueryProcess(osquerydPath, runsimple.WithStdout(&output))
	if err != nil {
		slogger.Log(ctx, slog.LevelError,
			"unable to create process",
			"err", err,
		)
		return
	}

	// This has a somewhat long timeout, in case there's a notarization fetch
	versionCtx, versionCancel := context.WithTimeout(ctx, 30*time.Second)
	defer versionCancel()

	osqErr := osq.RunVersion(versionCtx)

	// Output looks like `osquery version x.y.z`, so split on `version` and return the last part of the string
	parts := strings.SplitAfter(strings.TrimSpace(output.String()), "version")
	osquerydVersion := strings.TrimSpace(parts[len(parts)-1])

	if osqErr != nil {
		slogger.Log(ctx, slog.LevelError,
			"could not check osqueryd version",
			"output", osquerydVersion,
			"err", err,
			"osqueryd_path", osquerydPath,
		)
		return
	}

	// log the version to the knapsack
	k.SetCurrentRunningOsqueryVersion(osquerydVersion)

	slogger.Log(ctx, slog.LevelDebug,
		"checked osqueryd version",
		"osqueryd_version", osquerydVersion,
		"osqueryd_path", osquerydPath,
	)
}
