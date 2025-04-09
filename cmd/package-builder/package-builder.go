package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"github.com/kolide/launcher/pkg/packaging"
)

func runVersion(args []string) error {
	version.PrintFull()
	return nil
}

// Adapted from
// https://stackoverflow.com/questions/28322997/how-to-get-a-list-of-values-into-a-flag-in-golang/28323276#28323276
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, " ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var defaultWixPath = wix.FindWixInstall()

func runMake(args []string) error {
	flagset := flag.NewFlagSet("macos", flag.ExitOnError)
	var (
		flKolideUsage = flagset.Bool(
			"i-am-a-kolide-customer",
			false,
			"Certify that I am a Kolide customer and in compliance with the terms of the EE license",
		)
		flDebug = flagset.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flContainerTool = flagset.String(
			"container_tool",
			"docker",
			"container orchestration tool to build with ('docker', 'podman')",
		)
		flHostname = flagset.String(
			"hostname",
			env.String("HOSTNAME", ""),
			"the hostname of the gRPC server",
		)
		flPackageVersion = flagset.String(
			"package_version",
			env.String("PACKAGE_VERSION", ""),
			"the resultant package version. If left blank, auto detection will be attempted",
		)
		flBinRootDir = flagset.String(
			"bin_root_dir",
			"/usr/local",
			"the root directory path for the launcher on macOS and Linux",
		)
		flOsqueryVersion = flagset.String(
			"osquery_version",
			env.String("OSQUERY_VERSION", "stable"),
			"What TUF channel to download osquery from. Supports filesystem paths",
		)
		flLauncherVersion = flagset.String(
			"launcher_version",
			env.String("LAUNCHER_VERSION", "stable"),
			"What TUF channel to download launcher from. Supports filesystem paths",
		)
		flLauncherPath = flagset.String(
			"launcher_path",
			"",
			"Path of local launcher binary to use in packaging",
		)
		flLauncherArmPath = flagset.String(
			"launcher_arm_path",
			"",
			"Path of local launcher arm64 binary to use in packaging",
		)
		flExtensionVersion = flagset.String(
			"extension_version",
			env.String("EXTENSION_VERSION", "stable"),
			"What TUF channel to download the osquery extension from. Supports filesystem paths",
		)
		flEnrollSecret = flagset.String(
			"enroll_secret",
			env.String("ENROLL_SECRET", ""),
			"the string to be used as the server enrollment secret",
		)
		flSigningKey = flagset.String(
			"mac_package_signing_key",
			env.String("SIGNING_KEY", ""),
			"The name of the key that should be used for signing on apple platforms",
		)
		flTransport = flagset.String(
			"transport",
			env.String("TRANSPORT", ""),
			"Transport for launcher. Expected as grpc, jsonrpc. Default is up to launcher",
		)
		flInsecure = flagset.Bool(
			"insecure",
			env.Bool("INSECURE", false),
			"whether or not the launcher packages should invoke the launcher's --insecure flag",
		)
		flInsecureTransport = flagset.Bool(
			"insecure_transport",
			env.Bool("INSECURE_TRANSPORT", false),
			"whether or not the launcher packages should invoke the launcher's --insecure_transport flag",
		)
		flUpdateChannel = flagset.String(
			"update_channel",
			env.String("UPDATE_CHANNEL", ""),
			"the value that should be used when invoking the launcher's --update_channel flag. Autoupdates will be disabled unless this is specified",
		)
		flIdentifier = flagset.String(
			"identifier",
			env.String("IDENTIFIER", "launcher"),
			"the name of the directory that the launcher installation will shard into",
		)
		flOmitSecret = flagset.Bool(
			"omit_secret",
			env.Bool("OMIT_SECRET", false),
			"omit the enroll secret in the resultant package (default: false)",
		)
		flCertPins = flagset.String(
			"cert_pins",
			env.String("CERT_PINS", ""),
			"Comma separated, hex encoded SHA256 hashes of pinned subject public key info",
		)
		flRootPEM = flagset.String(
			"root_pem",
			env.String("ROOT_PEM", ""),
			"Path to PEM file including root certificates to verify against",
		)
		flOutputDir = flagset.String(
			"output_dir",
			env.String("OUTPUT_DIR", ""),
			"Directory to output package files to (default: random)",
		)
		flCacheDir = flagset.String(
			"cache_dir",
			env.String("CACHE_DIR", ""),
			"Directory to cache downloads in (default: random)",
		)
		flInitialRunner = flagset.Bool(
			"with_initial_runner",
			env.Bool("ENABLE_INITIAL_RUNNER", false),
			"Run differential queries from config ahead of scheduled interval.",
		)
		flTargets = flagset.String(
			"targets",
			env.String("TARGETS", defaultTargets()),
			"Target platforms to build. Specified in the form platform-init-package",
		)
		flTufURL = flagset.String(
			"tuf_url",
			env.String("TUF_URL", ""),
			"The TUF update server",
		)
		flMirrorURL = flagset.String(
			"mirror_url",
			env.String("MIRROR_URL", ""),
			"The mirror server for autoupdates",
		)
		flWixPath = flagset.String(
			"wix_path",
			defaultWixPath,
			fmt.Sprintf(`Location of wix binaries (default: "%s")`, defaultWixPath),
		)
		flWixSkipCleanup = flagset.Bool(
			"wix_skip_cleanup",
			false,
			"Keep wix temp files",
		)
		flDisableService = flagset.Bool(
			"disable_service",
			false,
			"Create persistence service in a disabled state",
		)
		flOsqueryFlags arrayFlags // set below with flagset.Var
	)
	flagset.Var(&flOsqueryFlags, "osquery_flag", "Flags to pass to osquery (possibly overriding Launcher defaults)")

	flagset.Usage = usageFor(flagset, "package-builder make [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	if !*flKolideUsage {
		fmt.Fprintf(os.Stderr, "\nThe Kolide Agent is for use with the Kolide Service.\n")
		fmt.Fprintf(os.Stderr, "See https://github.com/kolide/launcher/blob/main/ee/LICENSE\n")
		return errors.New("")
	}

	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	if *flDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	ctx := context.Background()
	ctx = ctxlog.NewContext(ctx, logger)

	if *flHostname == "" {
		return errors.New("hostname undefined")
	}

	// Validate that pinned certs are valid hex
	for _, pin := range strings.Split(*flCertPins, ",") {
		if _, err := hex.DecodeString(pin); err != nil {
			return fmt.Errorf("unable to parse cert pins: %w", err)
		}
	}

	// If we have a cacheDir, use it. Otherwise. set something random.
	cacheDir := *flCacheDir
	var err error
	if cacheDir == "" {
		cacheDir, err = os.MkdirTemp("", "download_cache")
		if err != nil {
			return fmt.Errorf("could not create temp dir for caching files: %w", err)
		}
		defer os.RemoveAll(cacheDir)
	}

	packageOptions := packaging.PackageOptions{
		PackageVersion:  *flPackageVersion,
		OsqueryVersion:  *flOsqueryVersion,
		OsqueryFlags:    flOsqueryFlags,
		LauncherVersion: *flLauncherVersion,
		LauncherPath:    *flLauncherPath,
		// LauncherArmPath can be used for windows arm64 packages when you want
		// to specify a local path to the launcher binary
		LauncherArmPath:   *flLauncherArmPath,
		ExtensionVersion:  *flExtensionVersion,
		Hostname:          *flHostname,
		Secret:            *flEnrollSecret,
		AppleSigningKey:   *flSigningKey,
		ContainerTool:     *flContainerTool,
		Transport:         *flTransport,
		Insecure:          *flInsecure,
		InsecureTransport: *flInsecureTransport,
		UpdateChannel:     *flUpdateChannel,
		InitialRunner:     *flInitialRunner,
		Identifier:        *flIdentifier,
		OmitSecret:        *flOmitSecret,
		CertPins:          *flCertPins,
		RootPEM:           *flRootPEM,
		BinRootDir:        *flBinRootDir,
		CacheDir:          cacheDir,
		TufServerURL:      *flTufURL,
		MirrorURL:         *flMirrorURL,
		WixPath:           *flWixPath,
		WixSkipCleanup:    *flWixSkipCleanup,
		DisableService:    *flDisableService,
	}

	outputDir := *flOutputDir

	// NOTE: if you are using docker-for-mac, you probably need to set the TMPDIR env to /tmp
	if outputDir == "" {
		var err error
		outputDir, err = os.MkdirTemp("", "launcher-package")
		if err != nil {
			return fmt.Errorf("making output dir: %w", err)
		}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	targets, err := getTargets(*flTargets)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if err := makeTarget(ctx, target, packageOptions, outputDir); err != nil {
			return fmt.Errorf("making target %s: %w", target.String(), err)
		}
	}

	fmt.Printf("Built packages in %s\n", outputDir)
	return nil
}

func makeTarget(ctx context.Context, target packaging.Target, packageOptions packaging.PackageOptions, outputDir string) error {
	outputFileName := fmt.Sprintf("launcher.%s.%s", target.String(), target.PkgExtension())
	outputFile, err := os.Create(filepath.Join(outputDir, outputFileName))
	if err != nil {
		return fmt.Errorf("failed to make package output file: %w", err)
	}
	defer outputFile.Close()

	if err := packageOptions.Build(ctx, outputFile, target); err != nil {
		return fmt.Errorf("could not generate packages: %w", err)
	}

	return nil
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE\n")
	fmt.Fprintf(os.Stderr, "  %s <mode> --help\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "MODES\n")
	fmt.Fprintf(os.Stderr, "  make          Generate a single launcher package for each platform\n")
	fmt.Fprintf(os.Stderr, "  list-targets  List all known build targets\n")
	fmt.Fprintf(os.Stderr, "  version       Print full version information\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "VERSION\n")
	fmt.Fprintf(os.Stderr, "  %s\n", version.Version().Version)
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher
	}

	var run func([]string) error
	switch strings.ToLower(os.Args[1]) {
	case "version":
		run = runVersion
	case "make":
		run = runMake
	case "list-targets":
		run = runListTargets
	default:
		usage()
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher
	}

	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher
	}
}

// defaultTargets sets the default target based on platform
func defaultTargets() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-service-msi"
	case "linux":
		return "linux-systemd-rpm,linux-systemd-deb"
	case "darwin":
		return "darwin-launchd-pkg"
	default:
		return ""
	}
}

// getTargets takes a string, and parses targets out of it. This
// encodes what the default mapping between human names and build
// targets is.
func getTargets(input string) ([]packaging.Target, error) {
	if input == "" {
		input = defaultTargets()
	}

	targets := []packaging.Target{}
	for _, targetString := range strings.Split(input, ",") {
		t := packaging.Target{}
		if err := t.Parse(targetString); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, nil
}
