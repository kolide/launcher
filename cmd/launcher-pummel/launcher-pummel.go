package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/simulator"
)

func main() {
	var (
		flVersion = flag.Bool(
			"version",
			env.Bool("VERSION", false),
			"Print version and exit",
		)
		flDebug = flag.Bool(
			"debug",
			env.Bool("DEBUG", false),
			"Print debug logs",
		)
		flJson = flag.Bool(
			"json",
			env.Bool("JSON", false),
			"Print logs in JSON format",
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
			"The enroll secret that is used in your environment",
		)
		flEnrollSecretPath = flag.String(
			"enroll_secret_path",
			env.String("ENROLL_SECRET_PATH", ""),
			"Optionally, the path to your enrollment secret",
		)
		flHosts = flag.String(
			"hosts",
			env.String("HOSTS", ""),
			"Comma-separated list of host type and quantity i.e.: linux:1000,macos:200",
		)
		flInsecureTLS = flag.Bool(
			"insecure",
			env.Bool("INSECURE", false),
			"Do not verify TLS certs for outgoing connections (default: false)",
		)
		flInsecureGRPC = flag.Bool(
			"insecure_grpc",
			env.Bool("INSECURE_GRPC", false),
			"Dial GRPC without a TLS config (default: false)",
		)
	)
	flag.Parse()

	var logger log.Logger

	if *flJson {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	} else {
		logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	}
	logger = log.With(logger,
		"ts", log.DefaultTimestampUTC,
		"component", "simulator",
	)
	logger = level.NewInjector(logger, level.InfoValue())
	logger = log.With(logger, "caller", log.DefaultCaller)

	if *flDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

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

	var enrollSecret string
	if *flEnrollSecret != "" {
		enrollSecret = *flEnrollSecret
	} else if *flEnrollSecretPath != "" {
		content, err := ioutil.ReadFile(*flEnrollSecretPath)
		if err != nil {
			logutil.Fatal(logger, "err", fmt.Errorf("could not read enroll_secret_path: %w", err), "enroll_secret_path", *flEnrollSecretPath)
		}
		enrollSecret = string(bytes.TrimSpace(content))
	}

	if len(enrollSecret) == 0 {
		logutil.Fatal(logger, "msg", "--enroll_secret cannot be empty")
	}

	level.Info(logger).Log(
		"msg", "starting load testing tool",
	)

	hostList := strings.Split(*flHosts, ",")
	if len(hostList) == 0 {
		logutil.Fatal(logger, "msg", "no hosts specified")
	}

	for _, hostSimulation := range hostList {
		simulationParts := strings.Split(hostSimulation, ":")
		if len(simulationParts) != 2 {
			logutil.Fatal(logger,
				"msg", "arguments should be of the form host_type:count",
				"arg", hostSimulation,
			)
		}
		hostType, countStr := simulationParts[0], simulationParts[1]

		count, err := strconv.Atoi(countStr)
		if err != nil {
			logutil.Fatal(logger,
				"msg", "unable to parse count",
				"arg", hostSimulation,
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

		opts := []simulator.SimulationOption{}
		if *flInsecureTLS {
			opts = append(opts, simulator.WithInsecure())
		}
		if *flInsecureGRPC {
			opts = append(opts, simulator.WithInsecureGrpc())
		}

		// Start hosts
		for i := 0; i < count; i++ {
			simulator.LaunchSimulation(
				logger,
				host,
				*flServerURL,
				fmt.Sprintf("%s_%d", hostType, i),
				enrollSecret,
				opts...,
			)
			time.Sleep(10 * time.Millisecond)
		}
	}

	level.Info(logger).Log(
		"msg", "all hosts started",
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
