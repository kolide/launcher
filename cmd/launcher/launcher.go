package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/kolide/launcher/osquery"
)

func main() {
	var (
		flBinPath = flag.String(
			"osqueryd",
			"/usr/local/kolide/bin/osqueryd",
			"path to osqueryd binary",
		)
	)
	flag.Parse()

	if platform, err := osquery.DetectPlatform(); err != nil {
		log.Fatalf("error detecting platform: %s\n", err)
	} else if platform != "darwin" {
		log.Fatalln("This tool only works on macOS right now")
	}

	workingDirectory := os.Getenv("KOLIDE_LAUNCHER_WORKING_DIR")
	if workingDirectory == "" {
		workingDirectory = os.TempDir()
	}

	if _, err := osquery.LaunchOsqueryInstance(*flBinPath, workingDirectory); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
