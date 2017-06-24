package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/kolide/launcher/osquery"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/kolide/updater"
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
}

// flagOrEnv accepts an option configured via a flag and an option configured
// via an environment variable and returns the correct value as determined by
// order of precedence
func flagOrEnv(flag, env string) string {
	if flag != "" {
		return flag
	}
	return env
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		flOsquerydPath = flag.String(
			"osqueryd_path",
			"",
			"path to osqueryd binary",
		)
		envOsquerydPath = os.Getenv("KOLIDE_LAUNCHER_OSQUERYD_PATH")
		flRootDirectory = flag.String(
			"root_directory",
			"",
			"path to the working directory where file artifacts can be stored",
		)
		envRootDirectory  = os.Getenv("KOLIDE_LAUNCHER_ROOT_DIRECTORY")
		flNotaryServerUrl = flag.String(
			"notary_url",
			"",
			"The URL of the notary update server",
		)
		envNotaryServerUrl = os.Getenv("KOLIDE_LAUNCHER_NOTARY_SERVER_URL")
	)
	flag.Parse()

	opts := &options{
		osquerydPath:    flagOrEnv(*flOsquerydPath, envOsquerydPath),
		rootDirectory:   flagOrEnv(*flRootDirectory, envRootDirectory),
		notaryServerUrl: flagOrEnv(*flNotaryServerUrl, envNotaryServerUrl),
	}

	// if an osqueryd path was not set, it's likely that we want to use the bundled
	// osqueryd path, but if it cannot be found, we will fail back to using an
	// osqueryd found in the path
	if opts.osquerydPath == "" {
		if _, err := os.Stat(defaultOsquerydPath); err != nil {
			opts.osquerydPath = defaultOsquerydPath
		} else if path, err := exec.LookPath("osqueryd"); err != nil {
			opts.osquerydPath = path
		}
	}

	// if a root directory was not set, we will fail back to using a temporary path
	if opts.rootDirectory == "" {
		opts.rootDirectory = os.TempDir()
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

	if opts.notaryServerUrl != "" {
		osqueryUpdater, err := updater.Start(updater.Settings{}, updateOsquery)
		if err != nil {
			log.Fatalf("Error launching osqueryd updater service %s\n", err)
		}
		defer osqueryUpdater.Stop()

		launcherUpdater, err := updater.Start(updater.Settings{}, updateLauncher)
		if err != nil {
			log.Fatalf("Error launching osqueryd updater service %s\n", err)
		}
		defer launcherUpdater.Stop()
	}

	if _, err := osquery.LaunchOsqueryInstance(
		osquery.WithOsquerydBinary(opts.osquerydPath),
		osquery.WithRootDirectory(opts.rootDirectory),
		osquery.WithConfigPluginFlag("kolide_grpc"),
		osquery.WithLoggerPluginFlag("kolide_grpc"),
		osquery.WithOsqueryExtensionPlugin(config.NewPlugin("kolide_grpc", osquery.GenerateConfigs)),
		osquery.WithOsqueryExtensionPlugin(logger.NewPlugin("kolide_grpc", osquery.LogString)),
	); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
