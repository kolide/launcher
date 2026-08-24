package launcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
)

var (
	// When launcher proper runs, it's expected that these defaults are their zero values
	// However, special launcher subcommands such as launcher doctor can override these
	DefaultRootDirectoryPath string
	DefaultEtcDirectoryPath  string
	DefaultBinDirectoryPath  string
	DefaultConfigFilePath    string
	DefaultAutoupdate        bool
)

// SetDefaultPaths populates the default file/dir paths
// call this before calling parseOptions if you want to assume these paths exist
func SetDefaultPaths() {
	DefaultRootDirectoryPath = DefaultPath(RootDirectory)
	DefaultEtcDirectoryPath = DefaultPath(EtcDirectory)
	DefaultBinDirectoryPath = DefaultPath(BinDirectory)
	DefaultConfigFilePath = DefaultPath(ConfigFile)
}

type defaultPath int

const (
	RootDirectory defaultPath = iota
	EtcDirectory
	WindowsConfigDirectory
	BinDirectory
	ConfigFile
	SecretFile
)

var likelyWindowsRootDirPaths = []string{
	"C:\\ProgramData\\Kolide\\Launcher-kolide-nababe-k2\\data",
	"C:\\Program Files\\Kolide\\Launcher-kolide-nababe-k2\\data",
	"C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data",
	"C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data",
}

func DefaultPath(path defaultPath) string {
	if runtime.GOOS == "windows" {
		switch path {
		case RootDirectory:
			return "C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data"
		case EtcDirectory, WindowsConfigDirectory:
			return "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\conf"
		case BinDirectory:
			return "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin"
		case ConfigFile:
			return filepath.Join(DefaultPath(WindowsConfigDirectory), "launcher.flags")
		case SecretFile:
			return filepath.Join(DefaultPath(WindowsConfigDirectory), "secret")
		default:
			return ""
		}
	}

	// not windows
	switch path {
	case RootDirectory:
		const defaultRootDir = "/var/kolide-k2/k2device.kolide.com"

		// see if default root dir exists, if not assume it's a preprod install
		if _, err := os.Stat(defaultRootDir); err != nil {
			return "/var/kolide-k2/k2device-preprod.kolide.com"
		}

		return defaultRootDir
	case EtcDirectory:
		return "/etc/kolide-k2/"
	case BinDirectory:
		return "/usr/local/kolide-k2/"
	case ConfigFile:
		return filepath.Join(DefaultPath(EtcDirectory), "launcher.flags")
	case SecretFile:
		return filepath.Join(DefaultPath(EtcDirectory), "secret")
	case WindowsConfigDirectory:
		// Not valid for non-Windows, but included for completeness
		fallthrough
	default:
		return ""
	}
}

type rootDirOverrideOpts struct {
	logger            *slog.Logger
	kolideServerURL   string
	packageIdentifier string
	isPrivileged      bool
	wellKnownRootDirs []string
}

// DetermineRootDirectoryOverride is used specifically for windows deployments to override the
// configured root directory if a well-known location we have access to containing a launcher DB already exists,
// and the configured root directory lacks a database. Always returns a directory.
func DetermineRootDirectoryOverride(logger *slog.Logger, optsRootDirectory, kolideServerURL, packageIdentifier string) string {
	if runtime.GOOS != "windows" {
		return optsRootDirectory
	}

	elevated, err := runningElevated() //nolint:staticcheck // Linux/MacOS unreachable, always errors
	if err != nil {                    //nolint:staticcheck
		logger.Log(context.TODO(), slog.LevelWarn,
			"failed to check if process is elevated, assuming privileged",
			"err", err,
		)
		// historically we never checked privileges here, so fall through
		elevated = true
	}

	return rootDirectoryOverride(optsRootDirectory, rootDirOverrideOpts{
		logger:            logger,
		kolideServerURL:   kolideServerURL,
		packageIdentifier: packageIdentifier,
		isPrivileged:      elevated,
		wellKnownRootDirs: likelyWindowsRootDirPaths,
	})
}

// rootDirectoryOverride holds the logic behind DetermineRootDirectoryOverride with stubs for testing.
// Only rootDirectory is required, every opts field defaults to the passthrough answer.
func rootDirectoryOverride(rootDirectory string, opts rootDirOverrideOpts) string {
	logger := opts.logger
	if logger == nil {
		logger = multislogger.NewNopLogger()
	}

	// don't mess with the path if this installation isn't pointing to a kolide server URL
	if !IsKolideHostedServerURL(opts.kolideServerURL) {
		return rootDirectory
	}

	// assume the default identifier if none is provided
	if strings.TrimSpace(opts.packageIdentifier) == "" {
		opts.packageIdentifier = DefaultLauncherIdentifier
	}

	dbLocation := filepath.Join(rootDirectory, "launcher.db")
	dbExists, err := nonEmptyFileExists(dbLocation)
	// If we get an unknown error, back out from making any options changes. This is an
	// unlikely path but doesn't feel right updating the rootDirectory without knowing what's going
	// on here
	if err != nil {
		logger.Log(context.TODO(), slog.LevelWarn,
			"failed to determine if an existing database is present in the root directory",
			"database", dbLocation,
			"err", err,
		)
		return rootDirectory
	}

	// database exists in configured path: not a fresh install, keep it
	if dbExists {
		return rootDirectory
	}

	// override paths are not usable for unprivileged users
	if !opts.isPrivileged {
		logger.Log(context.TODO(), slog.LevelWarn,
			"not running elevated, initializing launcher at configured root path and skipping well-known override locations",
			"root_directory", rootDirectory,
		)
		return rootDirectory
	}

	// we know this is a fresh install with no launcher.db in the configured root directory,
	// check likely locations and return updated rootDirectory if found
	for _, path := range opts.wellKnownRootDirs {
		if path == rootDirectory { // we already know this does not contain an enrolled DB
			continue
		}

		// a valid fallback path contains our identifier
		if !strings.Contains(path, opts.packageIdentifier) {
			continue
		}

		testingLocation := filepath.Join(path, "launcher.db")
		dbExists, err := nonEmptyFileExists(testingLocation)
		if err != nil {
			logger.Log(context.TODO(), slog.LevelWarn,
				"failed to determine if an existing database was present in a well-known location",
				"database", testingLocation,
				"err", err,
			)
			continue
		}

		if !dbExists {
			continue
		}

		logger.Log(context.TODO(), slog.LevelWarn,
			"overriding root directory to a well-known location",
			"original_root", rootDirectory,
			"new_root", path,
		)
		return path
	}

	// expected for devices that are truly installing from MSI for the first time
	return rootDirectory
}

func nonEmptyFileExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return fileInfo.Size() > 0, nil
}

// GetOriginalLauncherExecutablePath is a convenience function to determine and verify the location of
// the originally installed launcher executable. it uses the identifier to generate the expected path and
// verifies file presence before returning the path. this is currently in use for task installation
// on windows platforms
// Note: this will not work for NixOS, we should revisit if we end up with a use case there
func GetOriginalLauncherExecutablePath(identifier string) (string, error) {
	if strings.TrimSpace(identifier) == "" {
		identifier = DefaultLauncherIdentifier
	}

	var binDirBase string
	var launcherExeName string

	switch runtime.GOOS {
	case "windows":
		binDirBase = fmt.Sprintf(`C:\Program Files\Kolide\Launcher-%s\bin`, identifier)
		launcherExeName = "launcher.exe"
	default:
		binDirBase = fmt.Sprintf(`/usr/local/%s/bin`, identifier)
		launcherExeName = "launcher"
	}

	launcherBin := filepath.Join(binDirBase, launcherExeName)
	// do some basic sanity checking to prevent installation from a bad path
	if exists, err := nonEmptyFileExists(launcherBin); err != nil || !exists {
		return "", err
	}

	return launcherBin, nil
}
