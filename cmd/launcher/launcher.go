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
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
	"github.com/oklog/run"
	"github.com/pkg/errors"
)

// defaultOsquerydPath is the default path to the bundled osqueryd binary
const defaultOsquerydPath = "/usr/local/kolide/bin/osqueryd"

// runLauncher is the entry point into running launcher. It creates a
// rungroups with the various options, and goes! Autoupdate will kill the entire rungroup, and trigger FIXME
func runLauncher(ctx context.Context, cancel func(), opts *launcher.Options) error {
	logger := ctxlog.FromContext(ctx)

	// determine the root directory, create one if it's not provided
	rootDirectory := opts.RootDirectory
	if rootDirectory == "" {
		rootDirectory = filepath.Join(os.TempDir(), defaultRootDirectory)
		if _, err := os.Stat(rootDirectory); os.IsNotExist(err) {
			if err := os.Mkdir(rootDirectory, fs.DirMode); err != nil {
				return errors.Wrap(err, "creating temporary root directory")
			}
		}
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if err := os.MkdirAll(rootDirectory, 0700); err != nil {
		return errors.Wrap(err, "creating root directory")
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		return errors.Wrap(err, "detecting platform")
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

	// open the database for storing launcher data, we do it here because it's passed
	// to multiple actors
	db, err := bolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		return errors.Wrap(err, "open launcher db")
	}
	defer db.Close()

	if err := writePidFile(filepath.Join(rootDirectory, "launcher.pid")); err != nil {
		return errors.Wrap(err, "write launcher pid to file")
	}

	// create the certificate pool
	var rootPool *x509.CertPool
	if opts.RootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := ioutil.ReadFile(opts.RootPEM)
		if err != nil {
			return errors.Wrapf(err, "reading root certs PEM at path: %s", opts.RootPEM)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return errors.Errorf("found no valid certs in PEM at path: %s", opts.RootPEM)
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
		level.Info(logger).Log("msg", "interrupted", "err", err, "stack", fmt.Sprintf("%+v", err))
		cancel()
		close(sigChannel)
	})

	var client service.KolideService
	{
		switch opts.Transport {
		case "grpc":
			grpcConn, err := service.DialGRPC(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootPool, logger)
			if err != nil {
				return errors.Wrap(err, "dialing grpc server")
			}
			defer grpcConn.Close()
			client = service.NewGRPCClient(grpcConn, logger)
			queryTargeter := createQueryTargetUpdater(logger, db, grpcConn)
			runGroup.Add(queryTargeter.Execute, queryTargeter.Interrupt)
		case "jsonrpc":
			client = service.NewJSONRPCClient(opts.KolideServerURL, opts.InsecureTLS, opts.InsecureTransport, opts.CertPins, rootPool, logger)
		default:
			return errors.New("invalid transport option selected")
		}
	}

	// create the osquery extension for launcher. This is where osquery itself is launched.
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, db, logger, client, opts)
	if err != nil {
		return errors.Wrap(err, "create extension with runtime")
	}
	runGroup.Add(extension.Execute, extension.Interrupt)

	versionInfo := version.Version()
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	// If the control server has been opted-in to, run it
	if opts.Control {
		control, err := createControl(ctx, db, logger, opts)
		if err != nil {
			return errors.Wrap(err, "create control actor")
		}
		if control != nil {
			runGroup.Add(control.Execute, control.Interrupt)
		} else {
			level.Info(logger).Log("msg", "got nil control actor. Ignoring")
		}
	}

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if opts.Autoupdate {
		config := &updaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.AutoupdateInterval,
			UpdateChannel:      opts.UpdateChannel,
			NotaryURL:          opts.NotaryServerURL,
			MirrorURL:          opts.MirrorServerURL,
			NotaryPrefix:       opts.NotaryPrefix,
			HTTPClient:         httpClient,
			SigChannel:         sigChannel,
		}

		// create an updater for osquery
		osqueryUpdater, err := createUpdater(ctx, opts.OsquerydPath, runnerRestart, config)
		if err != nil {
			return errors.Wrap(err, "create osquery updater")
		}
		runGroup.Add(osqueryUpdater.Execute, osqueryUpdater.Interrupt)

		// create an updater for launcher
		launcherPath, err := os.Executable()
		if err != nil {
			logutil.Fatal(logger, "err", err)
		}
		launcherUpdater, err := createUpdater(
			ctx,
			launcherPath,
			updateFinalizer(logger, runnerShutdown),
			config,
		)
		if err != nil {
			return errors.Wrap(err, "create launcher updater")
		}
		runGroup.Add(launcherUpdater.Execute, launcherUpdater.Interrupt)
	}

	err = runGroup.Run()
	return errors.Wrap(err, "run service")
}

func writePidFile(path string) error {
	err := ioutil.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
	return errors.Wrap(err, "writing pidfile")
}
