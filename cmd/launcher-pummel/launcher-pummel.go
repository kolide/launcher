package main

import (
	"flag"
	"os"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	kolidelog "github.com/kolide/launcher/log"
)

func main() {
	var (
		flVersion = flag.Bool(
			"version",
			env.Bool("VERSION", false),
			"Print version and exit",
		)
	)
	flag.Parse()

	logger := kolidelog.NewLogger(os.Stderr)
	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}
	level.Info(logger).Log(
		"msg", "starting load testing tool",
	)
}
