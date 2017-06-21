package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/kolide/launcher/osquery"
)

const (
	binPath          = "/usr/local/kolide/bin/osqueryd"
	workingDirectory = "/var/kolide"
)

func main() {
	if platform, err := osquery.DetectPlatform(); err != nil {
		log.Fatalf("error detecting platform: %s\n", err)
	} else if platform != "darwin" {
		log.Fatalln("This tool only works on macOS right now")
	}

	if _, err := osquery.LaunchOsqueryInstance(binPath, workingDirectory); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
