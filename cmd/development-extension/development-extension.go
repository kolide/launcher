package main

import (
	"flag"
	"os"
	"time"

	"github.com/kolide/launcher/log"
	launcher "github.com/kolide/launcher/osquery"
	"github.com/kolide/osquery-go"
)

func main() {
	flSocket := flag.String("socket", "", "")
	flTimeout := flag.Int("timeout", 3, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	logger := log.NewLogger(os.Stderr)

	if *flSocket == "" {
		logger.Fatal("msg", "--socket flag cannot be empty")
	}

	server, err := osquery.NewExtensionManagerServer("dev_extension", *flSocket)
	if err != nil {
		logger.Fatal("err", err, "msg", "creating osquery extension server")
	}

	timeout := time.Duration(*flTimeout) * time.Second
	client, err := osquery.NewClient(*flSocket, timeout)
	if err != nil {
		logger.Fatal("err", err, "creating osquery extension client")
	}

	plugins := []osquery.OsqueryPlugin{}
	for _, tablePlugin := range launcher.PlatformTables(client, logger) {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		logger.Fatal("err", err)
	}
}
