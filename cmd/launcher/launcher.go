package main

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var (
	// applicationRoot is the path where the launcher filesystem root is located
	applicationRoot = "/usr/local/kolide/"
	// defaultOsquerydPath is the path to the bundled osqueryd binary
	defaultOsquerydPath = filepath.Join(applicationRoot, "bin/osqueryd")
)

// options is the set of configurable options that may be set when launching this
// program
type options struct {
	osquerydPath     string
	rootDirectory    string
	notaryServerURL  string
	kolideServerURL  string
	enrollSecret     string
	enrollSecretPath string
	printVersion     bool
	debug            bool
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		flDebug = flag.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flVersion = flag.Bool(
			"version",
			false,
			"print launcher version and exit",
		)
		flOsquerydPath = flag.String(
			"osqueryd_path",
			env.String("KOLIDE_LAUNCHER_OSQUERYD_PATH", ""),
			"path to osqueryd binary",
		)
		flRootDirectory = flag.String(
			"root_directory",
			env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", os.TempDir()),
			"path to the working directory where file artifacts can be stored",
		)
		flNotaryServerURL = flag.String(
			"notary_url",
			env.String("KOLIDE_LAUNCHER_NOTARY_SERVER_URL", ""),
			"The URL of the notary update server",
		)
		flKolideServerURL = flag.String(
			"kolide_url",
			env.String("KOLIDE_LAUNCHER_KOLIDE_URL", ""),
			"URL of the Kolide server to communicate with",
		)
		flEnrollSecret = flag.String(
			"enroll_secret",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", ""),
			"Enroll secret to authenticate with the Kolide server",
		)
		flEnrollSecretPath = flag.String(
			"enroll_secret_path",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET_PATH", ""),
			"Path to a file containing the enroll secret to authenticate with the Kolide server",
		)
	)
	flag.Parse()

	opts := &options{
		osquerydPath:     *flOsquerydPath,
		rootDirectory:    *flRootDirectory,
		notaryServerURL:  *flNotaryServerURL,
		printVersion:     *flVersion,
		kolideServerURL:  *flKolideServerURL,
		enrollSecret:     *flEnrollSecret,
		enrollSecretPath: *flEnrollSecretPath,
		debug:            *flDebug,
	}

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	if opts.osquerydPath == "" {
		if _, err := os.Stat(defaultOsquerydPath); err == nil {
			opts.osquerydPath = defaultOsquerydPath
		} else if path, err := exec.LookPath("osqueryd"); err == nil {
			opts.osquerydPath = path
		} else {
			return nil, errors.New("Could not find osqueryd binary")
		}
	}

	if opts.enrollSecret != "" && opts.enrollSecretPath != "" {
		return nil, errors.New("Both enroll_secret and enroll_secret_path were defined")
	}

	return opts, nil
}

// updateOsquery is the callback which handles new versions of the osqueryd
// binary
func updateOsquery(stagingDir string, err error) {
	return
}

// updateLauncher is the callback which handled new versions of the launcher
// binary
func updateLauncher(stagingDir string, err error) {
	return
}

func logFatal(logger log.Logger, args ...interface{}) {
	level.Info(logger).Log(args...)
	os.Exit(1)
}

func main() {
	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	if platform, err := osquery.DetectPlatform(); err != nil {
		logFatal(logger, "err", errors.Wrap(err, "detecting platform"))
	} else if platform != "darwin" {
		logFatal(logger, "err", "this tool only works on macOS right now")
	}

	opts, err := parseOptions()
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "invalid options"))
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	if opts.debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	versionInfo := version.Version()
	logFatal(logger, "msg", "started kolide launcher", "version", versionInfo.Version, "build", versionInfo.Revision)

	db, err := bolt.Open(filepath.Join(opts.rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	// TODO fix insecure
	conn, err := grpc.Dial(opts.kolideServerURL, grpc.WithInsecure(), grpc.WithTimeout(time.Second))
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "dialing grpc server"))
	}
	defer conn.Close()

	client := service.New(conn)

	var enrollSecret string
	if opts.enrollSecret != "" {
		enrollSecret = opts.enrollSecret
	} else if opts.enrollSecretPath != "" {
		content, err := ioutil.ReadFile(opts.enrollSecretPath)
		if err != nil {
			logFatal(logger, "err", errors.Wrap(err, "could not read enroll_secret_path"), "enroll_secret_path", opts.enrollSecretPath)
		}
		enrollSecret = string(content)
	}
	ext, err := osquery.NewExtension(client, db, osquery.ExtensionOpts{EnrollSecret: enrollSecret})
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "starting grpc extension"))
	}

	_, invalid, err := ext.Enroll(context.Background())
	if err != nil {
		logFatal(logger, "err", errors.Wrap(err, "enrolling host"))
	}
	if invalid {
		logFatal(logger, errors.Wrap(err, "invalid enroll secret"))
	}

	if _, err := osquery.LaunchOsqueryInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(opts.rootDirectory),
		osquery.WithConfigPluginFlag("kolide_grpc"),
		osquery.WithLoggerPluginFlag("kolide_grpc"),
		osquery.WithDistributedPluginFlag("kolide_grpc"),
		osquery.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		osquery.WithOsqueryExtensionPlugin(osquery_logger.NewPlugin("kolide_grpc", ext.LogString)),
		osquery.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		osquery.WithStdout(os.Stdout),
		osquery.WithStderr(os.Stderr),
	); err != nil {
		logFatal(logger, errors.Wrap(err, "launching osquery instance"))
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
