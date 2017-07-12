package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/osquery"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
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
	osquerydPath    string
	rootDirectory   string
	notaryServerUrl string
	kolideServerUrl string
	enrollSecret    string
	printVersion    bool
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
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
		flNotaryServerUrl = flag.String(
			"notary_url",
			env.String("KOLIDE_LAUNCHER_NOTARY_SERVER_URL", ""),
			"The URL of the notary update server",
		)
		flKolideServerUrl = flag.String(
			"kolide_url",
			env.String("KOLIDE_LAUNCHER_KOLIDE_URL", ""),
			"URL of the Kolide server to communicate with",
		)
		flEnrollSecret = flag.String(
			"enroll_secret",
			env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", ""),
			"Enroll secret to authenticate with the Kolide server",
		)
	)
	flag.Parse()

	opts := &options{
		osquerydPath:    *flOsquerydPath,
		rootDirectory:   *flRootDirectory,
		notaryServerUrl: *flNotaryServerUrl,
		printVersion:    *flVersion,
		kolideServerUrl: *flKolideServerUrl,
		enrollSecret:    *flEnrollSecret,
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
			log.Fatal("Could not find osqueryd binary")
		}
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

func main() {
	if platform, err := osquery.DetectPlatform(); err != nil {
		log.Fatalf("error detecting platform: %s\n", err)
	} else if platform != "darwin" {
		log.Fatalln("This tool only works on macOS right now")
	}

	opts, err := parseOptions()
	if err != nil {
		log.Fatalf("Unacceptable options: %s\n", err)
	}

	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	versionInfo := version.Version()
	log.Printf("Started kolide launcher, version %s, build %s\n", versionInfo.Version, versionInfo.Revision)

	ext, err := osquery.NewExtension(opts.kolideServerUrl, opts.enrollSecret, "bar_host")
	if err != nil {
		log.Fatalf("Error starting grpc extension: %s\n", err)
	}

	if _, err := osquery.LaunchOsqueryInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(opts.rootDirectory),
		osquery.WithConfigPluginFlag("kolide_grpc"),
		osquery.WithLoggerPluginFlag("kolide_grpc"),
		osquery.WithDistributedPluginFlag("kolide_grpc"),
		osquery.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", ext.GenerateConfigs)),
		osquery.WithOsqueryExtensionPlugin(logger.NewPlugin("kolide_grpc", ext.LogString)),
		osquery.WithOsqueryExtensionPlugin(distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)),
		osquery.WithStdout(os.Stdout),
		osquery.WithStderr(os.Stderr),
	); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
