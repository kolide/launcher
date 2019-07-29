package main

import (
	"context"
	"flag"
	"runtime"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/make"
)

func main() {
	buildAll := strings.Join([]string{
		"deps-go",
		"install-tools",
	}, ",")

	var (
		flTargets      = flag.String("targets", buildAll, "comma separated list of targets")
		flDebug        = flag.Bool("debug", false, "use a debug logger")
		flBuildARCH    = flag.String("arch", runtime.GOARCH, "Architecture to build for.")
		flNotaryServer = flag.String("notaryserver", autoupdate.DefaultNotary, "Notary server")
		flNotaryPrefix = flag.String("notaryprefix", autoupdate.DefaultNotaryPrefix, "Notary prefix (aka GUN)")
		flBuildOS      = flag.String("os", runtime.GOOS, "Operating system to build for.")
		flRace         = flag.Bool("race", false, "Build race-detector version of binaries.")
		flStatic       = flag.Bool("static", false, "Build a static binary.")
		flStampVersion = flag.Bool("linkstamp", false, "Add version info with ldflags.")
		flFakeData     = flag.Bool("fakedata", false, "Compile with build tags to falsify some data, like serial numbers")
	)
	flag.Parse()

	logger := logutil.NewCLILogger(*flDebug)
	ctx := context.Background()
	ctx = ctxlog.NewContext(ctx, logger)

	opts := []make.Option{
		make.WithOS(*flBuildOS),
		make.WithArch(*flBuildARCH),
	}
	if *flNotaryServer != "" {
		opts = append(opts, make.WithNotaryServer(*flNotaryServer))
	}
	if *flNotaryPrefix != "" {
		opts = append(opts, make.WithNotaryPrefix(*flNotaryPrefix))
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

	b, err := make.New(opts...)
	if err != nil {
		logutil.Fatal(logger, "msg", "Failed to create builder", "err", err)

	}

	targetSet := map[string]func(context.Context) error{
		"deps-go":         b.DepsGo,
		"install-tools":   b.InstallTools,
		"generate-tuf":    b.GenerateTUF,
		"launcher":        b.BuildCmd("./cmd/launcher", b.PlatformBinaryName("launcher")),
		"extension":       b.BuildCmd("./cmd/osquery-extension", b.PlatformExtensionName("osquery-extension")),
		"table-extension": b.BuildCmd("./cmd/launcher.ext", b.PlatformExtensionName("tables")),
		"grpc-extension":  b.BuildCmd("./cmd/grpc.ext", b.PlatformExtensionName("grpc")),
		"package-builder": b.BuildCmd("./cmd/package-builder", b.PlatformBinaryName("package-builder")),
		"make":            b.BuildCmd("./cmd/make", b.PlatformBinaryName("make")),
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
