package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
)

func main() {
	var (
		flSocketPath = flag.String("socket", "", "")
		flTimeout    = flag.Int("timeout", 2, "")
		flVerbose    = flag.Bool("verbose", false, "")
		flVersion    = flag.Bool("version", false, "Print  version and exit")
		_            = flag.Int("interval", 0, "")
	)
	flag.Parse()

	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	logger := logutil.NewServerLogger(*flVerbose)

	timeout := time.Duration(*flTimeout) * time.Second

	// allow for osqueryd to create the socket path
	time.Sleep(2 * time.Second)

	// create an extension server
	server, err := osquery.NewExtensionManagerServer(
		"com.kolide.standalone_extension",
		*flSocketPath,
		osquery.ServerTimeout(timeout),
	)
	if err != nil {
		level.Debug(logger).Log("err", err, "msg", "creating osquery extension server", "stack", fmt.Sprintf("%+v", err))
		logutil.Fatal(logger, "err", err, "msg", "creating osquery extension server")
	}

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		level.Debug(logger).Log("err", err, "creating osquery extension client", "stack", fmt.Sprintf("%+v", err))
		logutil.Fatal(logger, "err", err, "creating osquery extension client")
	}

	var plugins []osquery.OsqueryPlugin
	for _, tablePlugin := range table.PlatformTables(client, logger, "osqueryd") {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		level.Debug(logger).Log("err", err, "stack", fmt.Sprintf("%+v", err))
		logutil.Fatal(logger, "err", err)
	}
}
