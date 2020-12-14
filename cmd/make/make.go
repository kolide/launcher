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
		flGoPath       = fs.String("go", "", "Path for go binary. Will attempt auto detection")
		flRace         = fs.Bool("race", false, "Build race-detector version of binaries.")
		flStatic       = fs.Bool("static", false, "Build a static binary.")
		flStampVersion = fs.Bool("linkstamp", false, "Add version info with ldflags.")
		flFakeData     = fs.Bool("fakedata", false, "Compile with build tags to falsify some data, like serial numbers")
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

	b, err := make.New(opts...)
	if err != nil {
		logutil.Fatal(logger, "msg", "Failed to create builder", "err", err)

	}

	targetSet := map[string]func(context.Context) error{
		"deps-go":               b.DepsGo,
		"install-tools":         b.InstallTools,
		"generate-tuf":          b.GenerateTUF,
		"launcher":              b.BuildCmd("./cmd/launcher", fakeName("launcher", *flFakeData)),
		"osquery-extension.ext": b.BuildCmd("./cmd/osquery-extension", "osquery-extension.ext"),
		"tables.ext":            b.BuildCmd("./cmd/launcher.ext", "tables.ext"),
		"grpc.ext":              b.BuildCmd("./cmd/grpc.ext", "grpc.ext"),
		"package-builder":       b.BuildCmd("./cmd/package-builder", "package-builder"),
		"make":                  b.BuildCmd("./cmd/make", "make"),
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
