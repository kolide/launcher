package main

import (
	"flag"
	"time"

	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/kolide/osquery-go"
)

func main() {
	var (
		flSocketPath = flag.String("socket", "", "")
		flTimeout    = flag.Int("timeout", 0, "")
		flVerbose    = flag.Bool("verbose", false, "")
		_            = flag.Int("interval", 0, "")
	)
	flag.Parse()
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
		logutil.Fatal(logger, "err", err, "msg", "creating osquery extension server")
	}

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		logutil.Fatal(logger, "err", err, "creating osquery extension client")
	}

	var plugins []osquery.OsqueryPlugin
	for _, tablePlugin := range table.PlatformTables(client, logger) {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		logutil.Fatal(logger, "err", err)
	}
}
