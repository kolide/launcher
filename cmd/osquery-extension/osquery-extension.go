package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	flag.String("socket", "", "")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "%+v", os.Args)

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
