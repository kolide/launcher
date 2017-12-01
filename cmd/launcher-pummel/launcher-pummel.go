package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	kolidelog "github.com/kolide/launcher/log"
	"github.com/kolide/launcher/simulator"
)

func main() {
	var (
		flVersion = flag.Bool(
			"version",
			env.Bool("VERSION", false),
			"Print version and exit",
		)
		flHostPath = flag.String(
			"host_path",
			env.String("HOST_PATH", "simulator/hosts"),
			"Directory path for loading host yaml files",
		)
	)
	flag.Parse()

	logger := kolidelog.NewLogger(os.Stderr)
	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	hosts, err := simulator.LoadHosts(*flHostPath)
	if err != nil {
		level.Info(logger).Log(
			"msg", "error loading host definitions",
			"err", err,
		)
		os.Exit(1)
	}

	level.Info(logger).Log(
		"msg", "starting load testing tool",
	)

	if len(flag.Args()) == 0 {
		level.Info(logger).Log("msg", "no hosts specified")
		os.Exit(1)
	}

	for _, arg := range flag.Args() {
		s := strings.Split(arg, ":")
		if len(s) != 2 {
			level.Info(logger).Log(
				"msg", "arguments should be of the form host_type:count",
				"arg", arg,
			)
			os.Exit(1)
		}
		hostType, countStr := s[0], s[1]

		count, err := strconv.Atoi(countStr)
		if err != nil {
			level.Info(logger).Log(
				"msg", "unable to parse count",
				"arg", arg,
			)
			os.Exit(1)
		}

		host, ok := hosts[hostType]
		if !ok {
			level.Info(logger).Log(
				"msg", "unrecognized host type",
				"type", hostType,
			)
			os.Exit(1)
		}

		// Start hosts
		for i := 0; i < count; i++ {
			simulator.LaunchSimulation(
				host,
				"localhost:8080",
				fmt.Sprintf("%s_%d", hostType, i),
				"kRc8wX1klPTCqcqCKYAcc4lHfkzf51yb",
				simulator.WithInsecure(),
			)
		}
	}

	sleep := make(chan struct{})
	<-sleep
}
