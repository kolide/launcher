package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/debug"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/service"
	osquerygo "github.com/kolide/osquery-go"
	"github.com/oklog/run"
	"github.com/pkg/errors"
)

var (
	// applicationRoot is the path where the launcher filesystem root is located
	applicationRoot = "/usr/local/kolide/"
	// defaultOsquerydPath is the path to the bundled osqueryd binary
	defaultOsquerydPath = filepath.Join(applicationRoot, "bin/osqueryd")
)

type queryFile struct {
	Queries map[string]string `json:"queries"`
}

func commandUsage(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "  Usage:\n")
		fmt.Fprintf(os.Stderr, "    %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "  Flags:\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "    --%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func runQuery(args []string) error {
	flagset := flag.NewFlagSet("launcher query", flag.ExitOnError)
	var (
		flQueries = flagset.String(
			"queries",
			env.String("QUERIES", ""),
			"A file containing queries to run",
		)
		flSocket = flagset.String(
			"socket",
			env.String("SOCKET", ""),
			"The path to the socket",
		)
	)
	flagset.Usage = commandUsage(flagset, "launcher query")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	var queries queryFile

	if _, err := os.Stat(*flQueries); err == nil {
		data, err := ioutil.ReadFile(*flQueries)
		if err != nil {
			return errors.Wrap(err, "reading supplied queries file")
		}
		if err := json.Unmarshal(data, &queries); err != nil {
			return errors.Wrap(err, "unmarshalling queries file json")
		}
	}

	if *flQueries == "" {
		stdinQueries, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "reading stdin")
		}
		if err := json.Unmarshal(stdinQueries, &queries); err != nil {
			return errors.Wrap(err, "unmarshalling stdin queries json")
		}
	}

	if *flSocket == "" {
		return errors.New("--socket must be defined")
	}

	client, err := osquerygo.NewClient(*flSocket, 5*time.Second)
	if err != nil {
		return errors.Wrap(err, "opening osquery client connection on "+*flSocket)
	}
	defer client.Close()

	results := struct {
		Results map[string]interface{} `json:"results"`
	}{
		Results: map[string]interface{}{},
	}

	for name, query := range queries.Queries {
		resp, err := client.Query(query)
		if err != nil {
			return errors.Wrap(err, "running query")
		}

		if resp.Status.Code != int32(0) {
			fmt.Println("Error running query:", resp.Status.Message)
		}

		results.Results[name] = resp.Response
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	if err := enc.Encode(results); err != nil {
		return errors.Wrap(err, "encoding JSON query results")
	}

	return nil
}

func runSocket(args []string) error {
	flagset := flag.NewFlagSet("launcher socket", flag.ExitOnError)
	var (
		flPath = flagset.String(
			"path",
			env.String("SOCKET_PATH", filepath.Join(os.TempDir(), "osquery.sock")),
			"The path to the socket",
		)
	)
	flagset.Usage = commandUsage(flagset, "launcher socket")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Dir(*flPath)); os.IsNotExist(err) {
		if err := os.Mkdir(filepath.Dir(*flPath), fs.DirMode); err != nil {
			return errors.Wrap(err, "creating socket path base directory")
		}
	}

	runner, err := runtime.LaunchInstance(
		runtime.WithExtensionSocketPath(*flPath),
	)
	if err != nil {
		return errors.Wrap(err, "creating osquery instance")
	}

	fmt.Println(*flPath)

	// Wait forever
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, os.Kill, syscall.Signal(15))
	<-sig

	// allow for graceful termination.
	runner.Shutdown()

	return nil
}

func runSubcommands() error {
	var run func([]string) error
	switch os.Args[1] {
	case "socket":
		run = runSocket
	case "query":
		run = runQuery
	case "flare":
		run = runFlare
	}
	err := run(os.Args[2:])
	return errors.Wrapf(err, "running subcommand %s", os.Args[1])
}

// run the launcher daemon
func runLauncher(ctx context.Context, cancel func(), opts *options, logger log.Logger) error {
	// determine the root directory, create one if it's not provided
	rootDirectory := opts.rootDirectory
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
	if opts.insecureTLS {
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
	if opts.rootPEM != "" {
		rootPool = x509.NewCertPool()
		pemContents, err := ioutil.ReadFile(opts.rootPEM)
		if err != nil {
			return errors.Wrapf(err, "reading root certs PEM at path: %s", opts.rootPEM)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			return errors.Errorf("found no valid certs in PEM at path: %s", opts.rootPEM)
		}
	}

	// connect to the grpc server
	grpcConn, err := service.DialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, opts.certPins, rootPool, logger)
	if err != nil {
		return errors.Wrap(err, "dialing grpc server")
	}

	// create a rungroup for all the actors we create to allow for easy start/stop
	var runGroup run.Group

	// create the osquery extension for launcher
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, rootDirectory, db, logger, grpcConn, opts)
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

	queryTargeter := createQueryTargetUpdater(logger, db, grpcConn)
	runGroup.Add(queryTargeter.Execute, queryTargeter.Interrupt)

	// If the control server has been opted-in to, run it
	if opts.control {
		control, err := createControl(ctx, db, logger, opts)
		if err != nil {
			return errors.Wrap(err, "create conrol actor")
		}
		runGroup.Add(control.Execute, control.Interrupt)
	}

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if opts.autoupdate {
		config := &updaterConfig{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.autoupdateInterval,
			UpdateChannel:      opts.updateChannel,
			NotaryURL:          opts.notaryServerURL,
			MirrorURL:          opts.mirrorServerURL,
			HTTPClient:         httpClient,
		}

		// create an updater for osquery
		osqueryUpdater, err := createUpdater(ctx, opts.osquerydPath, runnerRestart, logger, config)
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
			launcherFinalizer(logger, runnerShutdown),
			logger,
			config,
		)
		if err != nil {
			return errors.Wrap(err, "create launcher updater")
		}
		runGroup.Add(launcherUpdater.Execute, launcherUpdater.Interrupt)
	}

	// Create the signal notifier and add it to the rungroup
	sig := make(chan os.Signal, 1)
	runGroup.Add(func() error {
		signal.Notify(sig, os.Interrupt)
		select {
		case <-sig:
			level.Info(logger).Log("msg", "beginnning shutdown")
			return nil
		}
	}, func(err error) {
		level.Info(logger).Log("msg", "interrupted", "err", err)
		cancel()
		close(sig)
	})

	err = runGroup.Run()
	return errors.Wrap(err, "run service")
}

func writePidFile(path string) error {
	err := ioutil.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
	return errors.Wrap(err, "writing pidfile")
}
