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
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/ee/agent/storage"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/timemachine"
	"github.com/kolide/launcher/ee/control/actionqueue"
	"github.com/kolide/launcher/ee/control/consumers/acceleratecontrolconsumer"
	"github.com/kolide/launcher/ee/control/consumers/flareconsumer"
	"github.com/kolide/launcher/ee/control/consumers/keyvalueconsumer"
	"github.com/kolide/launcher/ee/control/consumers/notificationconsumer"
	"github.com/kolide/launcher/ee/control/consumers/uninstallconsumer"
	"github.com/kolide/launcher/ee/debug/checkups"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/localserver"
	kolidelog "github.com/kolide/launcher/ee/log/osquerylogs"
	"github.com/kolide/launcher/ee/powereventwatcher"
	"github.com/kolide/launcher/ee/tuf"
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
	"github.com/kolide/launcher/pkg/osquery/table"
	"github.com/kolide/launcher/pkg/rungroup"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/kolide/launcher/pkg/traces/exporter"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquerylogger "github.com/osquery/osquery-go/plugin/logger"

	"go.etcd.io/bbolt"
)

const (
	// Subsystems that launcher listens for control server updates on
	agentFlagsSubsystemName  = "agent_flags"
	serverDataSubsystemName  = "kolide_server_data"
	desktopMenuSubsystemName = "kolide_desktop_menu"
	authTokensSubsystemName  = "auth_tokens"
	katcSubsystemName        = "katc_config" // Kolide ATC
)

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), multiSlogger, systemMultiSlogger *multislogger.MultiSlogger, opts *launcher.Options) error {
	initialTraceBuffer := exporter.NewInitialTraceBuffer()
	ctx, startupSpan := traces.StartSpan(ctx)

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
	boltOptions := &bbolt.Options{Timeout: time.Duration(30) * time.Second}
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

	go runOsqueryVersionCheck(ctx, slogger, k.LatestOsquerydPath(ctx))
	go timemachine.AddExclusions(ctx, k)

	if k.Debug() {
		// If we're in debug mode, then we assume we want to echo _all_ logs to stderr.
		k.AddSlogHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}))
	}

	// create a rungroup for all the actors we create to allow for easy start/stop
	runGroup := rungroup.NewRunGroup(slogger)

	// Need to set up the log shipper so that we can get the logger early
	// and pass it to the various systems.
	var logShipper *logshipper.LogShipper
	var traceExporter *exporter.TraceExporter
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

		traceExporter, err = exporter.NewTraceExporter(ctx, k, initialTraceBuffer)
		if err != nil {
			slogger.Log(ctx, slog.LevelDebug,
				"could not set up trace exporter",
				"err", err,
			)
		} else {
			runGroup.Add("traceExporter", traceExporter.Execute, traceExporter.Interrupt)
		}

		startupSpan.AddEvent("log_shipper_init_completed")
	}

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
	go checkpointer.Once(ctx)
	runGroup.Add("logcheckpoint", checkpointer.Run, checkpointer.Interrupt)

	// Create a channel for signals
	sigChannel := make(chan os.Signal, 1)

	// Add a rungroup to catch things on the sigChannel
	signalListener := newSignalListener(sigChannel, cancel, slogger)
	runGroup.Add("sigChannel", signalListener.Execute, signalListener.Interrupt)

	// For now, remediation is not performed -- we only log the hardware change.
	agent.DetectAndRemediateHardwareChange(ctx, k)

	powerEventWatcher, err := powereventwatcher.New(ctx, k, slogger)
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

	// init osquery instance history
	if err := osqueryInstanceHistory.InitHistory(k.OsqueryHistoryInstanceStore()); err != nil {
		return fmt.Errorf("error initializing osquery instance history: %w", err)
	}

	// create the osquery extension
	extension, err := createExtensionRuntime(ctx, k, client)
	if err != nil {
		return fmt.Errorf("create extension with runtime: %w", err)
	}
	runGroup.Add("osqueryExtension", extension.Execute, extension.Shutdown)
	// create the runner that will launch osquery
	osqueryRunner := osqueryruntime.New(
		k,
		osqueryruntime.WithKnapsack(k),
		osqueryruntime.WithRootDirectory(k.RootDirectory()),
		osqueryruntime.WithOsqueryExtensionPlugins(table.LauncherTables(k)...),
		osqueryruntime.WithSlogger(k.Slogger().With("component", "osquery_instance")),
		osqueryruntime.WithOsqueryVerbose(k.OsqueryVerbose()),
		osqueryruntime.WithOsqueryFlags(k.OsqueryFlags()),
		osqueryruntime.WithStdout(kolidelog.NewOsqueryLogAdapter(
			k.Slogger().With(
				"component", "osquery",
				"osqlevel", "stdout",
			),
			k.RootDirectory(),
			kolidelog.WithLevel(slog.LevelDebug),
		)),
		osqueryruntime.WithStderr(kolidelog.NewOsqueryLogAdapter(
			k.Slogger().With(
				"component", "osquery",
				"osqlevel", "stderr",
			),
			k.RootDirectory(),
			kolidelog.WithLevel(slog.LevelInfo),
		)),
		osqueryruntime.WithAugeasLensFunction(augeas.InstallLenses),
		osqueryruntime.WithUpdateDirectory(k.UpdateDirectory()),
		osqueryruntime.WithUpdateChannel(k.UpdateChannel()),
		osqueryruntime.WithConfigPluginFlag("kolide_grpc"),
		osqueryruntime.WithLoggerPluginFlag("kolide_grpc"),
		osqueryruntime.WithDistributedPluginFlag("kolide_grpc"),
		osqueryruntime.WithOsqueryExtensionPlugins(
			config.NewPlugin("kolide_grpc", extension.GenerateConfigs),
			distributed.NewPlugin("kolide_grpc", extension.GetQueries, extension.WriteResults),
			osquerylogger.NewPlugin("kolide_grpc", extension.LogString),
		),
	)
	runGroup.Add("osqueryRunner", osqueryRunner.Run, osqueryRunner.Interrupt)

	versionInfo := version.Version()
	k.SystemSlogger().Log(ctx, slog.LevelInfo,
		"started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	if traceExporter != nil {
		traceExporter.SetOsqueryClient(osqueryRunner)
	}

	// Create the control service and services that depend on it
	var runner *desktopRunner.DesktopUsersProcessesRunner
	var actionsQueue *actionqueue.ActionQueue
	if k.ControlServerURL() == "" {
		slogger.Log(ctx, slog.LevelDebug,
			"control server URL not set, will not create control service",
		)
	} else {
		controlService, err := createControlService(ctx, k.ControlStore(), k)
		if err != nil {
			return fmt.Errorf("failed to setup control service: %w", err)
		}
		runGroup.Add("controlService", controlService.ExecuteWithContext(ctx), controlService.Interrupt)

		// serverDataConsumer handles server data table updates
		controlService.RegisterConsumer(serverDataSubsystemName, keyvalueconsumer.New(k.ServerProvidedDataStore()))
		// agentFlagConsumer handles agent flags pushed from the control server
		controlService.RegisterConsumer(agentFlagsSubsystemName, keyvalueconsumer.New(flagController))
		// katcConfigConsumer handles updates to Kolide's custom ATC tables
		controlService.RegisterConsumer(katcSubsystemName, keyvalueconsumer.New(k.KatcConfigStore()))
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
	}

	runEECode := k.ControlServerURL() != "" || k.IAmBreakingEELicense()

	// at this moment, these values are the same. This variable is here to help humans parse what's happening
	runLocalServer := runEECode
	if runLocalServer {
		ls, err := localserver.New(
			ctx,
			k,
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
		mirrorClient := http.DefaultClient
		mirrorClient.Timeout = 8 * time.Minute // gives us extra time to avoid a timeout on download
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

// runOsqueryVersionCheck execs the osqueryd binary in the background when we're running
// on darwin. Operating on our theory that some startup delay issues for osquery might
// be due to the notarization check taking too long, we execute the binary here ahead
// of time in the hopes of getting the check out of the way. This is expected to be called
// from a goroutine, and thus does not return an error.
func runOsqueryVersionCheck(ctx context.Context, slogger *slog.Logger, osquerydPath string) {
	if runtime.GOOS != "darwin" {
		return
	}

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

	startTime := time.Now().UnixMilli()

	osqErr := osq.RunVersion(versionCtx)
	executionTimeMs := time.Now().UnixMilli() - startTime
	outTrimmed := strings.TrimSpace(output.String())

	if osqErr != nil {
		slogger.Log(ctx, slog.LevelError,
			"could not check osqueryd version",
			"output", outTrimmed,
			"err", err,
			"execution_time_ms", executionTimeMs,
			"osqueryd_path", osquerydPath,
		)
		return
	}

	slogger.Log(ctx, slog.LevelDebug,
		"checked osqueryd version",
		"osqueryd_version", outTrimmed,
		"execution_time_ms", executionTimeMs,
		"osqueryd_path", osquerydPath,
	)
}
