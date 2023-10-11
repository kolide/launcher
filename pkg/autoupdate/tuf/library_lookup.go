package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/peterbourgon/ff/v3"
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

var channelsUsingNewAutoupdater = map[string]bool{
	"nightly": true,
}

// CheckOutLatestWithoutConfig returns information about the latest downloaded executable for our binary,
// searching for launcher configuration values in its config file.
// For now, it is only available when launcher is on the nightly update channel.
func CheckOutLatestWithoutConfig(binary autoupdatableBinary, logger log.Logger) (*BinaryUpdateInfo, error) {
	logger = log.With(logger, "component", "tuf_library_lookup")
	cfg, err := getAutoupdateConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get autoupdate config: %w", err)
	}

	// Short-circuit lookup for local launcher test builds
	if binary == binaryLauncher && cfg.localDevelopmentPath != "" {
		return &BinaryUpdateInfo{Path: cfg.localDevelopmentPath}, nil
	}

	return CheckOutLatest(binary, cfg.rootDirectory, cfg.updateDirectory, cfg.channel, logger)
}

func UsingNewAutoupdater() bool {
	cfg, err := getAutoupdateConfig()
	if err != nil {
		return false
	}

	return ChannelUsesNewAutoupdater(cfg.channel)
}

// getAutoupdateConfig reads launcher's config file to determine the configuration values
// needed to work with the autoupdate library.
func getAutoupdateConfig() (*autoupdateConfig, error) {
	configFilePath := launcher.ConfigFilePath(os.Args[1:])
	if configFilePath == "" {
		return nil, errors.New("could not get config file path")
	}
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

// CheckOutLatest returns the path to the latest downloaded executable for our binary, as well
// as its version.
func CheckOutLatest(binary autoupdatableBinary, rootDirectory string, updateDirectory string, channel string, logger log.Logger) (*BinaryUpdateInfo, error) {
	// TODO: Remove this check once we decide to roll out the new autoupdater more broadly
	if !ChannelUsesNewAutoupdater(channel) {
		return nil, fmt.Errorf("not rolling out new TUF to channel %s that should still use legacy autoupdater", channel)
	}

	if updateDirectory == "" {
		updateDirectory = defaultLibraryDirectory(rootDirectory)
	}

	update, err := findExecutableFromRelease(binary, LocalTufDirectory(rootDirectory), channel, updateDirectory)
	if err == nil {
		level.Info(logger).Log("msg", "found executable matching current release", "path", update.Path, "version", update.Version)
		return update, nil
	}

	level.Info(logger).Log("msg", "could not find executable matching current release", "err", err)

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return mostRecentVersion(binary, updateDirectory)
}

func ChannelUsesNewAutoupdater(channel string) bool {
	_, ok := channelsUsingNewAutoupdater[channel]
	return ok
}

// findExecutableFromRelease looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path and version.
func findExecutableFromRelease(binary autoupdatableBinary, tufRepositoryLocation string, channel string, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
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

	targetName, _, err := findRelease(binary, targets, channel)
	if err != nil {
		return nil, fmt.Errorf("could not find release: %w", err)
	}

	targetPath, targetVersion := pathToTargetVersionExecutable(binary, targetName, baseUpdateDirectory)
	if autoupdate.CheckExecutable(context.TODO(), targetPath, "--version") != nil {
		return nil, fmt.Errorf("version %s from target %s is either originally installed version, not yet downloaded, or corrupted: %w", targetVersion, targetName, err)
	}

	return &BinaryUpdateInfo{
		Path:    targetPath,
		Version: targetVersion,
	}, nil
}

// mostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary, along with its version.
func mostRecentVersion(binary autoupdatableBinary, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	// Pull all available versions from library
	validVersionsInLibrary, _, err := sortedVersionsInLibrary(binary, baseUpdateDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// No valid versions in the library
	if len(validVersionsInLibrary) < 1 {
		return nil, fmt.Errorf("no versions of %s in library at %s", binary, baseUpdateDirectory)
	}

	// Versions are sorted in ascending order -- return the last one
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), mostRecentVersionInLibraryRaw)
	return &BinaryUpdateInfo{
		Path:    executableLocation(versionDir, binary),
		Version: mostRecentVersionInLibraryRaw,
	}, nil
}
