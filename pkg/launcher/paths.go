package launcher

import (
	"path/filepath"
	"runtime"
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

func DefaultPath(path defaultPath) string {
	if runtime.GOOS == "windows" {
		switch path {
		case RootDirectory:
			return "C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data"
		case WindowsConfigDirectory:
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
		return "/var/kolide-k2/k2device.kolide.com/"
	case EtcDirectory:
		return "/etc/kolide-k2/"
	case BinDirectory:
		return "/usr/local/kolide-k2/"
	case ConfigFile:
		return filepath.Join(DefaultPath(EtcDirectory), "launcher.flags")
	case SecretFile:
		return filepath.Join(DefaultPath(EtcDirectory), "secret")
	default:
		return ""
	}
}
