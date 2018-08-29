package main

import (
	"context"
	"crypto/tls"
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
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/debug"
	kolidelog "github.com/kolide/launcher/pkg/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime"
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

	// handle --version
	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	// handle --usage
	if opts.developerUsage {
		developerUsage()
		os.Exit(0)
	}

	// handle --debug
	if opts.debug {
		logger.AllowDebug()
	}
	debug.AttachLogToggle(logger, opts.debug)

	// determine the root directory, create one if it's not provided
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
		logger.Fatal("err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	// create a context for all the asynchronous stuff we are starting
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create a rungroup for all the actors we create to allow for easy start/stop
	var runGroup run.Group

	// create the osquery extension for launcher
	extension, runnerRestart, runnerShutdown, err := createExtensionRuntime(ctx, rootDirectory, db, logger, opts)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "creating extension and service"))
	}
	runGroup.Add(extension.Execute, extension.Interrupt)

	versionInfo := version.Version()
	level.Info(logger).Log(
		"msg", "started kolide launcher",
		"version", versionInfo.Version,
		"build", versionInfo.Revision,
	)

	// If the control server has been opted-in to, run it
	if opts.control {
		control, err := createControl(ctx, db, logger, opts)
		if err != nil {
			logger.Fatal(errors.Wrap(err, "creating control actor"))
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
			logger.Fatal(err)
		}
		runGroup.Add(osqueryUpdater.Execute, osqueryUpdater.Interrupt)

		// create an updater for launcher
		launcherPath, err := os.Executable()
		if err != nil {
			logger.Fatal(err)
		}
		launcherUpdater, err := createUpdater(
			ctx,
			launcherPath,
			launcherFinalizer(logger, runnerShutdown),
			logger,
			config,
		)
		if err != nil {
			logger.Fatal(err)
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

	// start the rungroup
	if err := runGroup.Run(); err != nil {
		logger.Fatal(err)
	}

}
