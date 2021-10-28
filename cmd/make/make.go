package main

import (
	"context"
	"flag"
	"os"
	"runtime"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/make"
	"github.com/peterbourgon/ff/v3"
)

func main() {
	buildAll := strings.Join([]string{
		"deps-go",
		"install-tools",
	}, ",")

	fs := flag.NewFlagSet("make", flag.ExitOnError)

	var (
		flTargets      = fs.String("targets", buildAll, "comma separated list of targets")
		flDebug        = fs.Bool("debug", false, "use a debug logger")
		flBuildARCH    = fs.String("arch", runtime.GOARCH, "Architecture to build for.")
		flBuildOS      = fs.String("os", runtime.GOOS, "Operating system to build for.")
		flGoPath       = fs.String("go", "", "Path for go binary. Will attempt auto detection.")
		flRace         = fs.Bool("race", false, "Build race-detector version of binaries.")
		flStatic       = fs.Bool("static", false, "Build a static binary.")
		flStampVersion = fs.Bool("linkstamp", false, "Add version info with ldflags.")
		flFakeData     = fs.Bool("fakedata", false, "Compile with build tags to falsify some data, like serial numbers")
		flGithubOutput = fs.Bool("github", os.Getenv("GITHUB_ACTIONS") != "", "Include github action output")
	)

	ffOpts := []ff.Option{
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithEnvVarPrefix("MAKE"),
	}

	if err := ff.Parse(fs, os.Args[1:], ffOpts...); err != nil {
		logger := logutil.NewCLILogger(true)
		logutil.Fatal(logger, "msg", "Error parsing flags", "err", err)
	}

	logger := logutil.NewCLILogger(*flDebug)
	ctx := context.Background()
	ctx = ctxlog.NewContext(ctx, logger)

	opts := []make.Option{
		make.WithOS(*flBuildOS),
		make.WithArch(*flBuildARCH),
	}
	if *flRace {
		opts = append(opts, make.WithRace())
	}
	if *flStatic {
		opts = append(opts, make.WithStatic())
	}
	if *flStampVersion {
		opts = append(opts, make.WithStampVersion())
	}
	if *flFakeData {
		opts = append(opts, make.WithFakeData())
	}

	if *flGoPath != "" {
		opts = append(opts, make.WithGoPath(*flGoPath))
	}

	if *flGithubOutput {
		opts = append(opts, make.WithGithubActionOutput())
	}

	optsWithCgo := append(opts, make.WithCgo())

	targetSet := map[string]func(context.Context) error{
		"deps-go":               make.New(opts...).DepsGo,
		"install-tools":         make.New(opts...).InstallTools,
		"generate-tuf":          make.New(opts...).GenerateTUF,
		"launcher":              make.New(optsWithCgo...).BuildCmd("./cmd/launcher", fakeName("launcher", *flFakeData)),
		"osquery-extension.ext": make.New(opts...).BuildCmd("./cmd/osquery-extension", "osquery-extension.ext"),
		"tables.ext":            make.New(optsWithCgo...).BuildCmd("./cmd/launcher.ext", "tables.ext"),
		"grpc.ext":              make.New(opts...).BuildCmd("./cmd/grpc.ext", "grpc.ext"),
		"package-builder":       make.New(opts...).BuildCmd("./cmd/package-builder", "package-builder"),
		"make":                  make.New(opts...).BuildCmd("./cmd/make", "make"),
	}

	if t := strings.Split(*flTargets, ","); len(t) != 0 && t[0] != "" {
		for _, target := range t {
			if fn, ok := targetSet[target]; ok {
				level.Debug(logger).Log("msg", "calling target", "target", target)
				if err := fn(ctx); err != nil {
					logutil.Fatal(logger, "msg", "Target Failed", "err", err, "target", target)
				}
			} else {
				logutil.Fatal(logger, "err", "target does not exist", "target", target)
			}
		}
	}
}

func fakeName(binName string, fake bool) string {
	if !fake {
		return binName
	}

	return binName + "-fake"
}
