package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/tools/packaging"
)

// options is the set of configurable options that may be set when launching
// this program
type options struct {
	osqueryVersion  string
	launcherVersion string
}

// parseOptions parses the options that may be configured via command-line flags
// and/or environment variables, determines order of precedence and returns a
// typed struct of options for further application use
func parseOptions() (*options, error) {
	var (
		flOsqueryVersion = flag.String(
			"osquery_version",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_OSQUERY_VERSION", ""),
			"the osquery version to include in the resultant packages",
		)
		flLauncherVersion = flag.String(
			"launcher_version",
			env.String("KOLIDE_LAUNCHER_PACKAGE_BUILDER_LAUNCHER_VERSION", ""),
			"the launcher version to include in the resultant packages",
		)
	)
	flag.Parse()

	opts := &options{
		osqueryVersion:  *flOsqueryVersion,
		launcherVersion: *flLauncherVersion,
	}

	if opts.osqueryVersion == "" {
		opts.osqueryVersion = "stable"
	}

	if opts.launcherVersion == "" {
		opts.launcherVersion = "stable"
	}

	return opts, nil
}

func main() {
	logger := log.NewLogfmtLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	opts, err := parseOptions()
	if err != nil {
		level.Error(logger).Log("error", fmt.Sprintf("Could not parse options: %s", err))
		os.Exit(1)
	}

	level.Debug(logger).Log(
		"osquery_version", opts.osqueryVersion,
		"launcher_version", opts.launcherVersion,
		"message", "finished parsing arguments",
	)

	firstID := 100001
	numberOfIDsToGenerate := 1000

	for id := firstID; id <= firstID+numberOfIDsToGenerate; id++ {
		tenant := packaging.Munemo(id)
		fmt.Println(tenant)
	}
}
