package main

import (
	"log"

	"github.com/kolide/agent/osquery"
)

func main() {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		log.Fatalln("error detecting platform:", err)
	}
	log.Println("detected platform: " + platform)
}
