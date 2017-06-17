package main

import (
	"flag"
	"fmt"
	"time"
)

func main() {
	socketPath := flag.String("socket", "", "path to osqueryd extension socket")
	flag.Int("timeout", 0, "")
	flag.Int("interval", 0, "")
	flag.Parse()

	fmt.Printf("{\"socketPath\": \"%s\"}\n", *socketPath)

	for {
		time.Sleep(time.Hour)
	}
}
