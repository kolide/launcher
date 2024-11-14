//go:build windows
// +build windows

package checkups

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/launcher"
	"golang.org/x/sys/windows/registry"
)

const (
	installerInfoRegistryKeyFmt = `Software\Kolide\Launcher\%s\%s`
	currentVersionKeyName       = `CurrentVersionNum`
	installedVersionKeyName     = `InstalledVersionNum`
	downloadPathKeyName         = `DownloadPath`
	identifierKeyName           = `Identifier`
	userKeyName                 = `User`
	versionKeyName              = `Version`
)

type installerInfo struct {
	// CurrentVersionNum is the numeric representation of our semver version, added at startup
	CurrentVersionNum uint64 `json:"current_version_num"`
	// InstalledVersionNum is the numeric representation of our semver version, added at install time
	InstalledVersionNum uint64 `json:"installed_version_num"`
	// Version is our semver string version representation, added at install time
	Version string `json:"installed_version"`
	// DownloadPath is the original location of the MSI used to install launcher
	DownloadPath string `json:"download_path"`
	Identifier   string `json:"identifier"`
	User         string `json:"user"`
}

func gatherInstallerInfo(z *zip.Writer, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		identifier = launcher.DefaultLauncherIdentifier
	}

	info := installerInfo{
		CurrentVersionNum: getDefaultRegistryIntValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, currentVersionKeyName),
		),
		InstalledVersionNum: getDefaultRegistryIntValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, installedVersionKeyName),
		),
		Version: getDefaultRegistryStringValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, versionKeyName),
		),
		DownloadPath: getDefaultRegistryStringValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, downloadPathKeyName),
		),
		Identifier: getDefaultRegistryStringValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, identifierKeyName),
		),
		User: getDefaultRegistryStringValue(
			fmt.Sprintf(installerInfoRegistryKeyFmt, identifier, userKeyName),
		),
	}

	infoJson, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return err
	}

	return addStreamToZip(z, "installer-info.json", time.Now(), bytes.NewReader(infoJson))
}

// getDefaultRegistryStringValue queries for the default registry value set at the provided path
// on the local machine. this is a best effort approach, grabbing whatever info we can. not all
// devices will have all paths/values present, errors are ignored
func getDefaultRegistryStringValue(path string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}

	defer key.Close()

	val, _, err := key.GetStringValue("")
	if err != nil {
		return ""
	}

	return val
}

// getDefaultRegistryIntValue performs the same function as getDefaultRegistryStringValue but
// is intended for integer (REG_DWORD) values
func getDefaultRegistryIntValue(path string) uint64 {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}

	defer key.Close()

	val, _, err := key.GetIntegerValue("")
	if err != nil {
		return 0
	}

	return val
}
