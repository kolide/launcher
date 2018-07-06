package main

import (
	"bytes"
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
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/autoupdate"
	"github.com/kolide/launcher/control"
	"github.com/kolide/launcher/debug"
	kolidelog "github.com/kolide/launcher/log"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/launcher/osquery/runtime"
	"github.com/kolide/launcher/osquery/table"
	"github.com/kolide/launcher/service"
	osquerygo "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
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
			return errors.Wrap(err, "unmarshaling queries file json")
		}
	}

	if *flQueries == "" {
		stdinQueries, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "reading stdin")
		}
		if err := json.Unmarshal(stdinQueries, &queries); err != nil {
			return errors.Wrap(err, "unmarshaling stdin queries json")
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

func main() {
	logger := kolidelog.NewLogger(os.Stderr)

	// if the launcher is being ran with a positional argument, handle that
	// argument. If a known positional argument is not supplied, fall-back to
	// running an osquery instance.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "socket":
			var args []string
			if len(os.Args) > 2 {
				args = os.Args[2:]
			}
			if err := runSocket(args); err != nil {
				logger.Fatal("err", errors.Wrap(err, "launching socket command"))
			}
			fmt.Println("\nexiting...")
			os.Exit(0)
		case "query":
			var args []string
			if len(os.Args) > 2 {
				args = os.Args[2:]
			}
			if err := runQuery(args); err != nil {
				logger.Fatal("err", errors.Wrap(err, "launching query command"))
			}
			os.Exit(0)
		}
	}

	opts, err := parseOptions()
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "invalid options"))
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	if opts.developerUsage {
		developerUsage()
		os.Exit(0)
	}

	if opts.debug {
		logger.AllowDebug()
	}
	debug.AttachLogToggle(logger, opts.debug)

	rootDirectory := opts.rootDirectory
	if rootDirectory == "" {
		rootDirectory = filepath.Join(os.TempDir(), defaultRootDirectory)
		if _, err := os.Stat(rootDirectory); os.IsNotExist(err) {
			if err := os.Mkdir(rootDirectory, fs.DirMode); err != nil {
				logger.Fatal("err", errors.Wrap(err, "creating temporary root directory"))
			}
		}
		level.Info(logger).Log(
			"msg", "using default system root directory",
			"path", rootDirectory,
		)
	}

	if err := os.MkdirAll(rootDirectory, 0700); err != nil {
		logger.Fatal("err", errors.Wrap(err, "creating root directory"))
	}

	if _, err := osquery.DetectPlatform(); err != nil {
		logger.Fatal("err", errors.Wrap(err, "detecting platform"))
	}

	debugAddrPath := filepath.Join(rootDirectory, "debug_addr")
	debug.AttachDebugHandler(debugAddrPath, logger)
	defer os.Remove(debugAddrPath)

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

	versionInfo := version.Version()
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	db, err := bolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	var rootPool *x509.CertPool
	if opts.rootPEM != "" {
		rootPool = x509.NewCertPool()

		pemContents, err := ioutil.ReadFile(opts.rootPEM)
		if err != nil {
			logger.Fatal(
				"err", errors.Wrap(err, "reading root certs PEM"),
				"path", opts.rootPEM,
			)
		}
		if ok := rootPool.AppendCertsFromPEM(pemContents); !ok {
			logger.Fatal(
				"err", "found no valid certs in PEM",
				"path", opts.rootPEM,
			)
		}
	}

	conn, err := service.DialGRPC(opts.kolideServerURL, opts.insecureTLS, opts.insecureGRPC, opts.certPins, rootPool, logger)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "dialing grpc server"))
	}
	defer conn.Close()

	client := service.New(conn, level.Debug(logger))

	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			logger.Fatal("err", errors.Wrap(err, "could not read enroll_secret_path"), "enroll_secret_path", opts.enrollSecretPath)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	extOpts := osquery.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: opts.loggingInterval,
	}

	ext, err := osquery.NewExtension(client, db, extOpts)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "starting grpc extension"))
	}

	osqueryLogger := &kolidelog.OsqueryLogAdapter{
		level.Debug(log.With(logger, "component", "osquery")),
	}

	// Start the osqueryd instance
	runner, err := runtime.LaunchInstance(
		runtime.WithOsquerydBinary(opts.osquerydPath),
		runtime.WithRootDirectory(rootDirectory),
		runtime.WithConfigPluginFlag("kolide_grpc"),
		runtime.WithLoggerPluginFlag("kolide_grpc"),
		runtime.WithDistributedPluginFlag("kolide_grpc"),
		runtime.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		runtime.WithOsqueryExtensionPlugin(osquery_logger.NewPlugin("kolide_grpc", ext.LogString)),
		runtime.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		runtime.WithOsqueryExtensionPlugin(table.LauncherIdentifierTable(db)),
		runtime.WithStdout(osqueryLogger),
		runtime.WithStderr(osqueryLogger),
		runtime.WithLogger(logger),
	)
	if err != nil {
		logger.Fatal(errors.Wrap(err, "launching osquery instance"))
	}

	// The runner allows querying the osqueryd instance from the extension.
	// Used by the Enroll method below to get initial enrollment details.
	ext.SetQuerier(runner)

	ctx := context.Background()
	_, invalid, err := ext.Enroll(ctx)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "enrolling host"))
	}
	if invalid {
		logger.Fatal(errors.Wrap(err, "invalid enroll secret"))
	}
	ext.Start()
	defer ext.Shutdown()

	// If the control server has been opted-in to, run it
	if opts.control {
		level.Debug(logger).Log("msg", "creating control client")

		controlOpts := []control.Option{
			control.WithLogger(logger),
			control.WithGetShellsInterval(opts.getShellsInterval),
		}
		if opts.insecureTLS {
			controlOpts = append(controlOpts, control.WithInsecureSkipVerify())
		}
		controlClient, err := control.NewControlClient(db, opts.controlServerURL, controlOpts...)
		if err != nil {
			logger.Fatal(errors.Wrap(err, "starting control client"))
		}

		controlClient.Start(ctx)
		defer controlClient.Stop()
	}

	// If the autoupdater is enabled, enable it for both osquery and launcher
	if opts.autoupdate {
		enabler := &updateEnabler{
			Logger:             logger,
			RootDirectory:      rootDirectory,
			AutoupdateInterval: opts.autoupdateInterval,
			NotaryURL:          opts.notaryServerURL,
			MirrorURL:          opts.mirrorServerURL,
			HttpClient:         httpClient,
		}

		stopOsquery, err := enabler.EnableBinary(
			opts.osquerydPath,
			autoupdate.WithFinalizer(runner.Restart),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logger.Fatal(err)
		}
		defer stopOsquery()

		launcherPath, err := os.Executable()
		if err != nil {
			logger.Fatal(err)
		}
		stopLauncher, err := enabler.EnableBinary(
			launcherPath,
			autoupdate.WithFinalizer(launcherFinalizer(logger, runner.Shutdown)),
			autoupdate.WithUpdateChannel(opts.updateChannel),
		)
		if err != nil {
			logger.Fatal(err)
		}
		defer stopLauncher()
	}

	// Wait forever
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt, os.Kill, syscall.Signal(15))
	<-sig

	// allow for graceful termination.
	runner.Shutdown()
}

func launcherFinalizer(logger log.Logger, shutdownOsquery func() error) func() error {
	return func() error {
		if err := shutdownOsquery(); err != nil {
			level.Info(logger).Log(
				"method", "launcherFinalizer",
				"err", err,
			)
		}
		// replace launcher
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			return errors.Wrap(err, "restarting launcher")
		}
		return nil
	}
}
