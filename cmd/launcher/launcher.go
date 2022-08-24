package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/internal"
	"github.com/kolide/launcher/cmd/launcher/internal/updater"
	systrayruntime "github.com/kolide/launcher/ee/systray/runtime"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/checkpoint"
	"github.com/kolide/launcher/pkg/osquery"
	osqueryInstanceHistory "github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/service"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! If autoupdate is
// enabled, the finalizers will trigger various restarts.
func runLauncher(ctx context.Context, cancel func(), opts *launcher.Options) error {
	logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)
	level.Debug(logger).Log("msg", "runLauncher starting")

	rootDirectory, err := initRootDirectory(logger, opts)
	if err != nil {
		return fmt.Errorf("init root directory: %w", err)
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		return fmt.Errorf("detecting platform: %w", err)
	}

	debugAddrPath := filepath.Join(rootDirectory, "debug_addr")
	debug.AttachDebugHandler(debugAddrPath, logger)
	defer os.Remove(debugAddrPath)

	httpClient := initHttpClient(opts)

	// open the database for storing launcher data, we do it here
	// because it's passed to multiple actors. Add a timeout to
	// this. Note that the timeout is documented as failing
	// unimplemented on windows, though empirically it seems to
	// work.
	db, err := initBoltDb(rootDirectory)
	if err != nil {
		return fmt.Errorf("init bolt db: %w", err)
	}
	defer db.Close()

	if err := writePidFile(filepath.Join(rootDirectory, "launcher.pid")); err != nil {
		return fmt.Errorf("writing launcher pid to file: %w", err)
	}

	// If we have successfully opened the DB, and written a pid,
	// we expect we're live. Record the version for osquery to
	// pickup
	internal.RecordLauncherVersion(rootDirectory)

	// Try to ensure useful info in the logs
	checkpoint.Run(logger, db, *opts)

	rootCertPool, err := createCertificatePool(opts)
	if err != nil {
		return fmt.Errorf("creating certificate pool: %w", err)
	}

	// create a rungroup for all the actors we create to allow for easy start/stop
	var runGroup run.Group

	sigChannel := handleSignals(logger, cancel, &runGroup)

	var client service.KolideService
	switch opts.Transport {
	case "grpc":
		grpcConn, err := service.DialGRPC(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootCertPool, logger)
		if err != nil {
			return fmt.Errorf("dialing grpc server: %w", err)
		}
		defer grpcConn.Close()
		client = service.NewGRPCClient(grpcConn, logger)
		queryTargeter := createQueryTargetUpdater(logger, db, grpcConn)
		runGroup.Add(queryTargeter.Execute, queryTargeter.Interrupt)
	case "jsonrpc":
		client = service.NewJSONRPCClient(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootCertPool, logger)
	case "osquery":
		client = service.NewNoopClient(logger)
	default:
		return errors.New("invalid transport option selected")
	}

	// init osquery instance history
	if err := osqueryInstanceHistory.InitHistory(db); err != nil {
		return fmt.Errorf("init osquery instance history: %w", err)
	}

	// create the osquery extension for launcher. This is where osquery itself is launched.
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, db, client, opts)
	if err != nil {
		return fmt.Errorf("creating extension with runtime: %w", err)
	}
	runGroup.Add(extension.Execute, extension.Interrupt)

	versionInfo := version.Version()
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	if err := setupControlServer(ctx, logger, db, &runGroup, opts); err != nil {
		return fmt.Errorf("setting up control server: %w", err)
	}

	systrayRunner := createSystrayRunner(logger, &runGroup, opts)

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
			return fmt.Errorf("creating osquery updater: %w", err)
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
				// reboot systray on auto update
				if systrayRunner != nil {
					systrayRunner.Interrupt(nil)
				}
				return runnerShutdown()
			}),
			launcherUpdaterconfig,
		)
		if err != nil {
			return fmt.Errorf("creating launcher updater: %w", err)
		}
		runGroup.Add(launcherUpdater.Execute, launcherUpdater.Interrupt)
	}

	err = runGroup.Run()
	return fmt.Errorf("running service: %w", err)
}

func writePidFile(path string) error {
	err := ioutil.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
	if err != nil {
		return fmt.Errorf("writing pid file: %w", err)
	}
	return nil
}

func initRootDirectory(logger log.Logger, opts *launcher.Options) (string, error) {
	if opts.RootDirectory != "" {
		return opts.RootDirectory, nil
	}

	rootDirectory := filepath.Join(os.TempDir(), defaultRootDirectory)
	if _, err := os.Stat(rootDirectory); os.IsNotExist(err) {
		if err := os.Mkdir(rootDirectory, fs.DirMode); err != nil {
			return "", fmt.Errorf("creating temporary root directory: %w", err)
		}
	}
	level.Info(logger).Log(
		"msg", "using default system root directory",
		"path", rootDirectory,
	)

	if err := os.MkdirAll(rootDirectory, 0700); err != nil {
		return "", fmt.Errorf("creating root directory: %w", err)
	}

	return rootDirectory, nil
}

func initHttpClient(opts *launcher.Options) *http.Client {
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
	return httpClient
}

func initBoltDb(dir string) (*bbolt.DB, error) {
	boltOptions := &bbolt.Options{Timeout: time.Duration(30) * time.Second}
	db, err := bbolt.Open(filepath.Join(dir, "launcher.db"), 0600, boltOptions)
	if err != nil {
		return nil, fmt.Errorf("opening launcher db: %w", err)
	}
	return db, nil
}

func createCertificatePool(opts *launcher.Options) (*x509.CertPool, error) {
	// create the certificate pool
	var rootPool *x509.CertPool
	if opts.RootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := ioutil.ReadFile(opts.RootPEM)
		if err != nil {
			return nil, fmt.Errorf("reading root certs PEM at path %s: %w", opts.RootPEM, err)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return nil, fmt.Errorf("appending root certs PEM at path %s: %w", opts.RootPEM, err)
		}
	}
	return rootPool, nil
}

func handleSignals(logger log.Logger, cancel func(), runGroup *run.Group) chan os.Signal {
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

	return sigChannel
}

func createSystrayRunner(logger log.Logger, runGroup *run.Group, opts *launcher.Options) *systrayruntime.SystrayUsersProcessesRunner {
	if (opts.KolideServerURL == "k2device-preprod.kolide.com" || opts.KolideServerURL == "localhost:3443") && runtime.GOOS == "darwin" {
		systrayRunner := systrayruntime.New(logger, time.Second*5)
		runGroup.Add(systrayRunner.Execute, systrayRunner.Interrupt)
		return systrayRunner
	}

	return nil
}

func setupControlServer(ctx context.Context, logger log.Logger, db *bbolt.DB, runGroup *run.Group, opts *launcher.Options) error {
	if !opts.Control {
		return nil
	}

	// If the control server has been opted-in to, run it
	control, err := createControl(ctx, db, logger, opts)
	if err != nil {
		return fmt.Errorf("creating control actor: %w", err)
	}
	if control != nil {
		runGroup.Add(control.Execute, control.Interrupt)
	} else {
		level.Info(logger).Log("msg", "got nil control actor. Ignoring")
	}
	return nil
}
