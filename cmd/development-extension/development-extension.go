package main

import (
	"flag"
	"log"
	"time"

	launcher "github.com/kolide/launcher/osquery"
	"github.com/kolide/osquery-go"
)

func main() {
	flSocket := flag.String("socket", "", "")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	if *flSocket == "" {
		log.Fatalln("--socket flag cannot be empty")
	}

	server, err := osquery.NewExtensionManagerServer("dev_extension", *flSocket)
	if err != nil {
		log.Fatalf("Error creating osquery extension server: %s\n", err)
	}

	client, err := osquery.NewClient(*flSocket, 3*time.Second)
	if err != nil {
		log.Fatalf("Error creating osquery extension client: %s\n", err)
	}

	plugins := []osquery.OsqueryPlugin{}
	for _, tablePlugin := range launcher.PlatformTables(client) {
		plugins = append(plugins, tablePlugin)
	}
	server.RegisterPlugin(plugins...)

	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
