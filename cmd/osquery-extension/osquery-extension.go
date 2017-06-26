package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	flag.String("socket", "", "")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	fmt.Printf("%+v", os.Args)

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		for {
			s := <-signalChannel
			fmt.Println("Received %+v", s)
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
