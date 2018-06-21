package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kolide/launcher/log"
	"github.com/kolide/launcher/osquery/table"
	osquery "github.com/kolide/osquery-go"
)

func main() {
	// if the extension is launched with a positional argument, handle that entrypoint first.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "cf_preference":
			if len(os.Args) != 4 {
				fmt.Println("the cf_preference command requires 2 arguments", len(os.Args))
				os.Exit(2)
			}
			key, domain := os.Args[2], os.Args[3]
			table.PrintPreferenceValue(key, domain)
			os.Exit(0)
		}
	}

	// standard entrypoint to the extension called by osqueryd
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
	for _, tablePlugin := range table.PlatformTables(client, logger) {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		logger.Fatal("err", err)
	}
}
