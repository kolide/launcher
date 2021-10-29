package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/kolide/kit/version"
)

func main() {
	var (
		_         = flag.Bool("verbose", false, "")
		_         = flag.Int("interval", 0, "")
		_         = flag.Int("timeout", 0, "")
		_         = flag.String("socket", "", "")
		flVersion = flag.Bool("version", false, "Print  version and exit")
	)
	flag.Parse()

	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "%+v", os.Args)

	go monitorForParent()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

// continuously monitor for ppid and exit if osqueryd is no longer the parent process.
// because osqueryd is always the process starting the extension, when osqueryd is killed this process should also be cleaned up.
// sometimes the termination is not clean, causing this process to remain running, which sometimes prevents osqueryd from properly restarting.
// https://github.com/kolide/launcher/issues/341
func monitorForParent() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	f := func() {
		ppid := os.Getppid()
		if ppid <= 1 {
			fmt.Println("extension process no longer owned by osqueryd, quitting")
			os.Exit(1)
		}
	}

	f()

	select {
	case <-ticker.C:
		f()
	}
}
