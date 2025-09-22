package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// DetermineRootDirectoryOverride is used specifically for windows deployments to override the
// configured root directory if another well known location containing a launcher DB already exists
// This is used by ParseOptions which doesn't have access to a logger, we should add more logging here
// when we have that available
func DetermineRootDirectoryOverride(optsRootDirectory, kolideServerURL, packageIdentifier string) string {
	if runtime.GOOS != "windows" {
		return optsRootDirectory
	}

	// don't mess with the path if this installation isn't pointing to a kolide server URL
	if !IsKolideHostedServerURL(kolideServerURL) {
		return optsRootDirectory
	}

	// assume the default identifier if none is provided
	if strings.TrimSpace(packageIdentifier) == "" {
		packageIdentifier = DefaultLauncherIdentifier
	}

	optsDBLocation := filepath.Join(optsRootDirectory, "launcher.db")
	dbExists, err := nonEmptyFileExists(optsDBLocation)
	// If we get an unknown error, back out from making any options changes. This is an
	// unlikely path but doesn't feel right updating the rootDirectory without knowing what's going
	// on here
	if err != nil {
		// we should add logs here when available - revisit with https://github.com/kolide/launcher/issues/1698
		return optsRootDirectory
	}

	// database already exists in configured root directory, keep that
	if dbExists {
		return optsRootDirectory
	}

	// we know this is a fresh install with no launcher.db in the configured root directory,
	// check likely locations and return updated rootDirectory if found
	for _, path := range likelyWindowsRootDirPaths {
		if path == optsRootDirectory { // we already know this does not contain an enrolled DB
			continue
		}

		// the fallaback path MUST contain the identifier
		if !strings.Contains(path, packageIdentifier) {
			continue
		}

		testingLocation := filepath.Join(path, "launcher.db")
		dbExists, err := nonEmptyFileExists(testingLocation)
		if err == nil && dbExists {
			return path
		}

		if err != nil {
			// we should add logs here when available - revisit with https://github.com/kolide/launcher/issues/1698
			continue
		}
	}

	// if all else fails, return the originally configured rootDirectory -
	// this is expected for devices that are truly installing from MSI for the first time
	return optsRootDirectory
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
