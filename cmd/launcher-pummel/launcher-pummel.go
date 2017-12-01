package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
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
		flServerURL = flag.String(
			"server_url",
			env.String("SERVER_URL", "localhost:8080"),
			"URL of gRPC server to load test",
		)
		flEnrollSecret = flag.String(
			"enroll_secret",
			env.String("ENROLL_SECRET", ""),
			"Enroll secret for host enrollment (required)",
		)
	)
	flag.Parse()

	logger := newLogger(os.Stderr)
	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	hosts, err := simulator.LoadHosts(*flHostPath, logger)
	if err != nil {
		logutil.Fatal(logger,
			"msg", "error loading host definitions",
			"err", err,
		)
	}

	if len(*flEnrollSecret) == 0 {
		logutil.Fatal(logger, "msg", "--enroll_secret cannot be empty")
	}

	level.Info(logger).Log(
		"msg", "starting load testing tool",
	)

	if len(flag.Args()) == 0 {
		logutil.Fatal(logger, "msg", "no hosts specified")
	}

	for _, arg := range flag.Args() {
		s := strings.Split(arg, ":")
		if len(s) != 2 {
			logutil.Fatal(logger,
				"msg", "arguments should be of the form host_type:count",
				"arg", arg,
			)
		}
		hostType, countStr := s[0], s[1]

		count, err := strconv.Atoi(countStr)
		if err != nil {
			logutil.Fatal(logger,
				"msg", "unable to parse count",
				"arg", arg,
			)
		}

		host, ok := hosts[hostType]
		if !ok {
			logutil.Fatal(logger,
				"msg", "unrecognized host type",
				"type", hostType,
			)
		}

		level.Info(logger).Log(
			"msg", "starting hosts",
			"count", count,
		)
		// Start hosts
		for i := 0; i < count; i++ {
			simulator.LaunchSimulation(
				&host,
				*flServerURL,
				fmt.Sprintf("%s_%d", hostType, i),
				*flEnrollSecret,
				simulator.WithInsecure(),
			)
			time.Sleep(10 * time.Millisecond)
		}
	}
	level.Info(logger).Log(
		"msg", "all hosts started",
	)

	sleep := make(chan struct{})
	<-sleep
}

func newLogger(w io.Writer) log.Logger {
	logger := log.NewJSONLogger(log.NewSyncWriter(w))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "component", "simulator")
	logger = level.NewInjector(logger, level.InfoValue())
	logger = log.With(logger, "caller", log.DefaultCaller)
	return logger
}
