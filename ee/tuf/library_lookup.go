package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/peterbourgon/ff/v3"
	"github.com/spf13/pflag"
)

type BinaryUpdateInfo struct {
	Path    string
	Version string
}

type autoupdateConfig struct {
	rootDirectory        string
	updateDirectory      string
	channel              string
	localDevelopmentPath string
}

// CheckOutLatestWithoutConfig returns information about the latest downloaded executable for our binary,
// searching for launcher configuration values in its config file.
// For now, it is only available when launcher is on the nightly update channel.
func CheckOutLatestWithoutConfig(binary autoupdatableBinary, logger log.Logger) (*BinaryUpdateInfo, error) {
	logger = log.With(logger, "component", "tuf_library_lookup")
	cfg, err := getAutoupdateConfig(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("could not get autoupdate config: %w", err)
	}

	// Short-circuit lookup for local launcher test builds
	if binary == binaryLauncher && cfg.localDevelopmentPath != "" {
		return &BinaryUpdateInfo{Path: cfg.localDevelopmentPath}, nil
	}

	return CheckOutLatest(context.Background(), binary, cfg.rootDirectory, cfg.updateDirectory, cfg.channel, logger)
}

// ShouldUseNewAutoupdater retrieves the root directory from the command-line args
// (either set via command-line arg, or set in the config file indicated by the --config arg),
// and then looks up whether the new autoupdater has been enabled for this installation.
func ShouldUseNewAutoupdater(ctx context.Context) bool {
	cfg, err := getAutoupdateConfig(os.Args[1:])
	if err != nil {
		return false
	}

	return usingNewAutoupdater(ctx, cfg.rootDirectory)
}

// getAutoupdateConfig pulls the configuration values necessary to work with the autoupdate library
// from either the given args or from the config file.
func getAutoupdateConfig(args []string) (*autoupdateConfig, error) {
	// pflag, while mostly great for our usecase here, expects getopt-style flags, which means
	// it doesn't support the Golang standard of using single and double dashes interchangeably
	// for flags. (e.g., pflag cannot parse `-config`, but Golang treats `-config` the same as
	// `--config`.) This transforms all single-dash args to double-dashes so that pflag can parse
	// them as expected.
	argsToParse := make([]string, len(args))
	for i := 0; i < len(args); i += 1 {
		if strings.HasPrefix(args[i], "-") && !strings.HasPrefix(args[i], "--") {
			argsToParse[i] = "-" + args[i]
			continue
		}

		argsToParse[i] = args[i]
	}

	// Create a flagset with options that are relevant to autoupdate only.
	// Ensure that we won't fail out when we see other command-line options.
	pflagSet := pflag.NewFlagSet("autoupdate options", pflag.ContinueOnError)
	pflagSet.ParseErrorsWhitelist = pflag.ParseErrorsWhitelist{UnknownFlags: true}

	// Extract the config flag plus the autoupdate flags
	var flConfigFilePath, flRootDirectory, flUpdateDirectory, flUpdateChannel, flLocalDevelopmentPath string
	pflagSet.StringVar(&flConfigFilePath, "config", "", "")
	pflagSet.StringVar(&flRootDirectory, "root_directory", "", "")
	pflagSet.StringVar(&flUpdateDirectory, "update_directory", "", "")
	pflagSet.StringVar(&flUpdateChannel, "update_channel", "", "")
	pflagSet.StringVar(&flLocalDevelopmentPath, "localdev_path", "", "")

	if err := pflagSet.Parse(argsToParse); err != nil {
		return nil, fmt.Errorf("parsing command-line flags: %w", err)
	}

	// If the config file wasn't set AND the other critical flags weren't set, fall back
	// to looking in the default config flag file location. (The update directory and local
	// development path are both optional flags and not critical to library lookup
	// functionality.) We expect all the flags to be set either via config flag (flConfigFilePath
	// is set) or via command line (flRootDirectory and flUpdateChannel are set), but do not
	// support a mix of both for this usage.
	if flConfigFilePath == "" && flRootDirectory == "" && flUpdateChannel == "" {
		return getAutoupdateConfigFromFile(launcher.ConfigFilePath(argsToParse))
	}

	if flConfigFilePath != "" {
		return getAutoupdateConfigFromFile(flConfigFilePath)
	}

	cfg := &autoupdateConfig{
		rootDirectory:        flRootDirectory,
		updateDirectory:      flUpdateDirectory,
		channel:              flUpdateChannel,
		localDevelopmentPath: flLocalDevelopmentPath,
	}

	return cfg, nil
}

// getAutoupdateConfigFromFile reads launcher's config file to determine the configuration values
// needed to work with the autoupdate library.
func getAutoupdateConfigFromFile(configFilePath string) (*autoupdateConfig, error) {
	if _, err := os.Stat(configFilePath); err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("could not read config file because it does not exist at %s: %w", configFilePath, err)
	}

	cfgFileHandle, err := os.Open(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("could not open config file %s for reading: %w", configFilePath, err)
	}
	defer cfgFileHandle.Close()

	cfg := &autoupdateConfig{}
	if err := ff.PlainParser(cfgFileHandle, func(name, value string) error {
		switch name {
		case "root_directory":
			cfg.rootDirectory = value
		case "update_directory":
			cfg.updateDirectory = value
		case "update_channel":
			cfg.channel = value
		case "localdev_path":
			cfg.localDevelopmentPath = value
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("could not parse config file %s: %w", configFilePath, err)
	}

	return cfg, nil
}

// usingNewAutoupdater reads from the shared startup settings db to see whether the
// UseTUFAutoupdater flag has been set for this installation.
func usingNewAutoupdater(ctx context.Context, rootDirectory string) bool {
	r, err := startupsettings.OpenReader(ctx, rootDirectory)
	if err != nil {
		// For now, default to not using the new autoupdater
		return false
	}
	defer r.Close()

	enabledStr, err := r.Get(keys.UseTUFAutoupdater.String())
	if err != nil {
		// For now, default to not using the new autoupdater
		return false
	}

	return flags.StringToBool(enabledStr)
}

// CheckOutLatest returns the path to the latest downloaded executable for our binary, as well
// as its version.
func CheckOutLatest(ctx context.Context, binary autoupdatableBinary, rootDirectory string, updateDirectory string, channel string, logger log.Logger) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx, "binary", string(binary))
	defer span.End()

	// TODO: Remove this check once we decide to roll out the new autoupdater more broadly
	if !usingNewAutoupdater(ctx, rootDirectory) {
		span.AddEvent("not_using_new_autoupdater")
		return nil, errors.New("not using new autoupdater yet")
	}

	if updateDirectory == "" {
		updateDirectory = DefaultLibraryDirectory(rootDirectory)
	}

	update, err := findExecutableFromRelease(ctx, binary, LocalTufDirectory(rootDirectory), channel, updateDirectory)
	if err == nil {
		span.AddEvent("found_latest_from_release")
		level.Info(logger).Log("msg", "found executable matching current release", "path", update.Path, "version", update.Version)
		return update, nil
	}

	level.Info(logger).Log("msg", "could not find executable matching current release", "err", err)

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return mostRecentVersion(ctx, binary, updateDirectory)
}

// findExecutableFromRelease looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path and version.
func findExecutableFromRelease(ctx context.Context, binary autoupdatableBinary, tufRepositoryLocation string, channel string, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Initialize a read-only TUF metadata client to parse the data we already have downloaded about releases.
	metadataClient, err := readOnlyTufMetadataClient(tufRepositoryLocation)
	if err != nil {
		return nil, errors.New("could not initialize TUF client, cannot find release")
	}

	// From already-downloaded metadata, look for the release version
	targets, err := metadataClient.Targets()
	if err != nil {
		return nil, fmt.Errorf("could not get target: %w", err)
	}

	targetName, _, err := findRelease(ctx, binary, targets, channel)
	if err != nil {
		return nil, fmt.Errorf("could not find release: %w", err)
	}

	targetPath, targetVersion := pathToTargetVersionExecutable(binary, targetName, baseUpdateDirectory)
	if err := autoupdate.CheckExecutable(ctx, targetPath, "--version"); err != nil {
		traces.SetError(span, err)
		return nil, fmt.Errorf("version %s from target %s at %s is either originally installed version, not yet downloaded, or corrupted: %w", targetVersion, targetName, targetPath, err)
	}

	return &BinaryUpdateInfo{
		Path:    targetPath,
		Version: targetVersion,
	}, nil
}

// mostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary, along with its version.
func mostRecentVersion(ctx context.Context, binary autoupdatableBinary, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Pull all available versions from library
	validVersionsInLibrary, _, err := sortedVersionsInLibrary(ctx, binary, baseUpdateDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// No valid versions in the library
	if len(validVersionsInLibrary) < 1 {
		return nil, fmt.Errorf("no versions of %s in library at %s", binary, baseUpdateDirectory)
	}

	span.AddEvent("found_latest_from_library")

	// Versions are sorted in ascending order -- return the last one
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), mostRecentVersionInLibraryRaw)
	return &BinaryUpdateInfo{
		Path:    executableLocation(versionDir, binary),
		Version: mostRecentVersionInLibraryRaw,
	}, nil
}
