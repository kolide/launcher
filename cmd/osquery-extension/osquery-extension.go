package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func main() {
	flag.String("socket", "", "")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "%+v", os.Args)

	go monitorForParent()

	sig := make(chan os.Signal)
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
