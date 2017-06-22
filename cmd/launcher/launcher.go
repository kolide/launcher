package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/kolide/launcher/osquery"
	"github.com/kolide/updater"
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

	notary := os.Getenv("KOLIDE_LAUNCHER_NOTARY_URL")
	if notary != "" {
		osqueryUpdater, err := updater.Start(updater.Settings{}, updateOsquery)
		if err != nil {
			log.Fatalf("Error launching osqueryd updater service %s\n", err)
		}
		defer osqueryUpdater.Stop()

		launcherUpdater, err := updater.Start(updater.Settings{}, updateLauncher)
		if err != nil {
			log.Fatalf("Error launching osqueryd updater service %s\n", err)
		}
		defer launcherUpdater.Stop()
	}

	if _, err := osquery.LaunchOsqueryInstance(*flBinPath, workingDirectory); err != nil {
		log.Fatalf("Error launching osquery instance: %s", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

func updateOsquery(stagingDir string, err error) {
	return
}

func updateLauncher(stagingDir string, err error) {
	return
}
