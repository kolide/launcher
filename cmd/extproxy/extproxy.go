package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	flag.String("socket", "", "")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Bool("verbose", false, "")
	flag.Parse()

	fmt.Printf("%+v", os.Args)

	for {
		time.Sleep(time.Hour)
	}
}
