package main

import (
	"flag"
	"os"
	"time"

	"github.com/kolide/launcher/log"
	launcher "github.com/kolide/launcher/osquery"
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
	logger := log.NewLogger(os.Stderr)
	if *flVerbose {
		logger.AllowDebug()
	}

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
		logger.Fatal("err", err, "msg", "creating osquery extension server")
	}

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		logger.Fatal("err", err, "creating osquery extension client")
	}

	var plugins []osquery.OsqueryPlugin
	for _, tablePlugin := range launcher.PlatformTables(client, logger) {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		logger.Fatal("err", err)
	}
}
