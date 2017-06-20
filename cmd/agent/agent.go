package main

import (
	"fmt"
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

	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		if i%5 == 0 {
			fmt.Printf("Sleeping for %d more seconds...\n", 30-i)
		}
	}

	if err := osq.Kill(); err != nil {
		log.Fatalf("Unable to kill osqueryd: %s", err)
	}
}
