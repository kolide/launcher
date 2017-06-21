package main

import (
	"log"
	"os"

	"github.com/kolide/agent/osquery"
)

func main() {
	if platform, err := osquery.DetectPlatform(); err != nil {
		log.Fatalln("error detecting platform:", err)
	} else if platform != "darwin" {
		log.Fatalln("This tool only works on macOS right now")
	}

	if _, err := osquery.LaunchOsqueryInstance("/usr/local/kolide-corp/bin/osqueryd", os.TempDir()); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}
}
