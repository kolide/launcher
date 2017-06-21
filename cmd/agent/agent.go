package main

import (
	"log"
	"os"
	"time"

	"github.com/kolide/agent/osquery"
)

func main() {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		log.Fatalln("error detecting platform:", err)
	}
	if platform != "darwin" {
		log.Fatalln("This tool only works on macOS right now")
	}

	osq, err := osquery.LaunchOsqueryInstance("/usr/local/kolide-corp/bin/osqueryd", os.TempDir())
	if err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	time.Sleep(10 * time.Second)
	log.Println("Quitting!")

	if err := osq.Kill(); err != nil {
		log.Fatalf("Unable to kill osqueryd: %s", err)
	}
}
