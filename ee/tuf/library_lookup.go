package tuf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/startupsettings"
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
	hostname             string
	identifier           string
}

// CheckOutLatestWithoutConfig returns information about the latest downloaded executable for our binary,
// searching for launcher configuration values in its config file.
func CheckOutLatestWithoutConfig(binary autoupdatableBinary, slogger *slog.Logger) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	slogger = slogger.With("component", "tuf_library_lookup")
	cfg, err := getAutoupdateConfig(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("could not get autoupdate config: %w", err)
	}

	// Short-circuit lookup for local launcher test builds
	if binary == binaryLauncher && cfg.localDevelopmentPath != "" {
		return &BinaryUpdateInfo{Path: cfg.localDevelopmentPath}, nil
	}

	// check for old root directories before returning final config in case we've stomped over with windows MSI install
	updatedRootDirectory := launcher.DetermineRootDirectoryOverride(cfg.rootDirectory, cfg.hostname, cfg.identifier)
	if updatedRootDirectory != cfg.rootDirectory {
		slogger.Log(context.TODO(), slog.LevelInfo,
			"old root directory contents detected, overriding for autoupdate config",
			"opts_root_directory", cfg.rootDirectory,
			"updated_root_directory", updatedRootDirectory,
		)

		cfg.rootDirectory = updatedRootDirectory
	}

	// Get update channel from startup settings
	pinnedVersion, updateChannel, err := getUpdateSettingsFromStartupSettings(ctx, slogger, binary, cfg.rootDirectory)
	if err != nil {
		slogger.Log(ctx, slog.LevelWarn,
			"could not get startup settings",
			"err", err,
		)
	}
	// Default to config update channel if not set in startup settings
	if updateChannel == "" {
		updateChannel = cfg.channel
	}

	return CheckOutLatest(ctx, binary, cfg.rootDirectory, cfg.updateDirectory, pinnedVersion, updateChannel, slogger)
}

// getUpdateSettingsFromStartupSettings queries the startup settings database to fetch the
// pinned version and update channel. This accounts for e.g. the control server sending down
// a particular value for the update channel, overriding the config file.
func getUpdateSettingsFromStartupSettings(ctx context.Context, slogger *slog.Logger, binary autoupdatableBinary, rootDirectory string) (string, string, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	r, err := startupsettings.OpenReader(ctx, slogger, rootDirectory)
	if err != nil {
		return "", "", fmt.Errorf("opening startupsettings reader: %w", err)
	}
	defer r.Close()

	var pinnedVersion string
	switch binary {
	case binaryLauncher:
		pinnedVersion, _ = r.Get(keys.PinnedLauncherVersion.String())
	case binaryOsqueryd:
		pinnedVersion, _ = r.Get(keys.PinnedOsquerydVersion.String())
	}

	updateChannel, _ := r.Get(keys.UpdateChannel.String())

	return pinnedVersion, updateChannel, nil
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
	var flConfigFilePath, flRootDirectory, flUpdateDirectory, flUpdateChannel, flLocalDevelopmentPath, flHostname, flIdentifier string
	pflagSet.StringVar(&flConfigFilePath, "config", "", "")
	pflagSet.StringVar(&flRootDirectory, "root_directory", "", "")
	pflagSet.StringVar(&flUpdateDirectory, "update_directory", "", "")
	pflagSet.StringVar(&flUpdateChannel, "update_channel", "", "")
	pflagSet.StringVar(&flLocalDevelopmentPath, "localdev_path", "", "")
	pflagSet.StringVar(&flHostname, "hostname", "", "")
	pflagSet.StringVar(&flIdentifier, "identifier", "", "")

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
		identifier:           flIdentifier,
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
		case "hostname":
			cfg.hostname = value
		case "identifier":
			cfg.identifier = value
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("could not parse config file %s: %w", configFilePath, err)
	}

	return cfg, nil
}

// CheckOutLatest returns the path to the latest downloaded executable for our binary, as well
// as its version.
func CheckOutLatest(ctx context.Context, binary autoupdatableBinary, rootDirectory string,
	updateDirectory string, pinnedVersion string, channel string, slogger *slog.Logger) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx, "binary", string(binary))
	defer span.End()
	slogger = slogger.With("binary", string(binary), "update_channel", channel, "pinned_version", pinnedVersion)

	if updateDirectory == "" {
		updateDirectory = DefaultLibraryDirectory(rootDirectory)
	}

	update, err := findExecutable(ctx, binary, LocalTufDirectory(rootDirectory), pinnedVersion, channel, updateDirectory, slogger)
	if err == nil {
		span.AddEvent("found_latest")
		slogger.Log(ctx, slog.LevelInfo,
			"found executable matching current release or pinned version",
			"executable_path", update.Path,
			"executable_version", update.Version,
		)
		return update, nil
	}

	slogger.Log(ctx, slog.LevelInfo,
		"could not find executable matching current release",
		"err", err,
	)

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return mostRecentVersion(ctx, slogger, binary, updateDirectory, channel)
}

// findExecutable looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path and version.
func findExecutable(ctx context.Context, binary autoupdatableBinary, tufRepositoryLocation string, pinnedVersion string, channel string, baseUpdateDirectory string, slogger *slog.Logger) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Initialize a read-only TUF metadata client to parse the data we already have downloaded about releases.
	metadataClient, err := readOnlyTufMetadataClient(tufRepositoryLocation)
	if err != nil {
		return nil, fmt.Errorf("could not initialize TUF client, cannot find release: %w", err)
	}

	// From already-downloaded metadata, look for the release version
	targets, err := metadataClient.Targets()
	if err != nil {
		return nil, fmt.Errorf("could not get targets: %w", err)
	}
	if len(targets) == 0 {
		return nil, errors.New("no local TUF metadata available -- likely new install")
	}

	targetName, _, err := findTarget(ctx, binary, targets, pinnedVersion, channel, slogger)
	if err != nil {
		return nil, fmt.Errorf("could not find release: %w", err)
	}

	targetPath, targetVersion := pathToTargetVersionExecutable(binary, targetName, baseUpdateDirectory)
	if _, err := os.Stat(targetPath); err != nil && errors.Is(err, os.ErrNotExist) {
		traces.SetError(span, err)
		return nil, fmt.Errorf("version %s from target %s at %s is either originally installed version or not yet downloaded", targetVersion, targetName, targetPath)
	}
	if err := CheckExecutable(ctx, slogger, targetPath, "--version"); err != nil {
		traces.SetError(span, err)
		return nil, fmt.Errorf("version %s from target %s at %s is corrupted: %w", targetVersion, targetName, targetPath, err)
	}

	return &BinaryUpdateInfo{
		Path:    targetPath,
		Version: trimVersionString(targetVersion),
	}, nil
}

// mostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary, along with its version.
func mostRecentVersion(ctx context.Context, slogger *slog.Logger, binary autoupdatableBinary, baseUpdateDirectory, channel string) (*BinaryUpdateInfo, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Pull all available versions from library
	validVersionsInLibrary, _, err := sortedVersionsInLibrary(ctx, slogger, binary, baseUpdateDirectory)
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

	// We rolled out TUF more broadly beginning in v1.4.1. Don't select versions earlier than that.
	if binary == binaryLauncher && channel == "stable" {
		supportsTuf, err := launcherVersionSupportsTuf(mostRecentVersionInLibraryRaw)
		if err != nil {
			return nil, fmt.Errorf("could not determine if launcher version %s supports TUF: %w", mostRecentVersionInLibraryRaw, err)
		}
		if !supportsTuf {
			return nil, fmt.Errorf("most recent version %s for binary launcher is not newer than required %s", mostRecentVersionInLibraryRaw, tufVersionMinimum.String())
		}
	}

	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), mostRecentVersionInLibraryRaw)
	return &BinaryUpdateInfo{
		Path:    executableLocation(versionDir, binary),
		Version: trimVersionString(mostRecentVersionInLibraryRaw),
	}, nil
}

func trimVersionString(version string) string {
	parts := strings.Fields(version)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return version
}
