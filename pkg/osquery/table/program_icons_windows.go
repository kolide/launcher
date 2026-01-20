package table

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc64"
	"image/png"
	"log/slog"
	"os"

	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/mat/besticon/ico"
	"github.com/nfnt/resize"
	"github.com/osquery/osquery-go/plugin/table"
	"golang.org/x/sys/windows/registry"
)

var crcTable = crc64.MakeTable(crc64.ECMA)

type icon struct {
	base64 string
	hash   uint64
}

func ProgramIcons(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("version"),
		table.TextColumn("icon"),
		table.TextColumn("hash"),
	}
	return tablewrapper.New(flags, slogger, "kolide_program_icons", columns, generateProgramIcons)
}

func ProgramIconChecksums(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("version"),
		table.TextColumn("hash"),
	}
	return tablewrapper.New(flags, slogger, "kolide_program_icon_checksums", columns, generateProgramIconChecksums)
}

func generateProgramIcons(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	programNames := programNameLookup(queryContext)

	results = append(results, generateUninstallerProgramIcons(programNames, true)...)
	results = append(results, generateInstallersProgramIcons(programNames, true)...)

	return results, nil
}

func generateProgramIconChecksums(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	programNames := programNameLookup(queryContext)

	results = append(results, generateUninstallerProgramIcons(programNames, false)...)
	results = append(results, generateInstallersProgramIcons(programNames, false)...)

	return results, nil
}

// programNameLookup retrieves the query constraints for the "name" column,
// and transforms them into a map for easy lookup.
func programNameLookup(queryContext table.QueryContext) map[string]struct{} {
	programNames := tablehelpers.GetConstraints(queryContext, "name")
	programNamesMap := make(map[string]struct{})
	for _, programName := range programNames {
		programNamesMap[programName] = struct{}{}
	}
	return programNamesMap
}

func generateUninstallerProgramIcons(programNames map[string]struct{}, includeIcon bool) []map[string]string {
	var uninstallerIcons []map[string]string

	uninstallRegPaths := map[registry.Key][]string{
		registry.LOCAL_MACHINE: append(expandRegistryKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*`),
			expandRegistryKey(registry.LOCAL_MACHINE, `\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*`)...),
		registry.USERS: expandRegistryKey(registry.USERS, `*\Software\Microsoft\Windows\CurrentVersion\Uninstall\*`),
	}

	for key, paths := range uninstallRegPaths {
		for _, path := range paths {
			iconPath, name, version, err := getRegistryKeyDisplayData(key, path)
			if err != nil {
				continue
			}

			// If we're filtering on program names, check for that
			if len(programNames) > 0 {
				if _, found := programNames[name]; !found {
					continue
				}
			}

			icon, err := parseIcoFile(iconPath, includeIcon)
			if err != nil {
				continue
			}

			result := map[string]string{
				"hash":    fmt.Sprintf("%x", icon.hash),
				"name":    name,
				"version": version,
			}

			if includeIcon {
				result["icon"] = icon.base64
			}

			uninstallerIcons = append(uninstallerIcons, result)
		}
	}
	return uninstallerIcons
}

func getRegistryKeyDisplayData(key registry.Key, path string) (string, string, string, error) {
	key, err := registry.OpenKey(key, path, registry.READ)
	if err != nil {
		return "", "", "", fmt.Errorf("opening key: %w", err)
	}
	defer key.Close()

	iconPath, _, err := key.GetStringValue("DisplayIcon")
	if err != nil {
		return "", "", "", fmt.Errorf("getting DisplayIcon: %w", err)
	}

	name, _, err := key.GetStringValue("DisplayName")
	if err != nil {
		return "", "", "", fmt.Errorf("getting DisplayName: %w", err)
	}

	version, _, err := key.GetStringValue("DisplayVersion")
	if err != nil {
		return "", "", "", fmt.Errorf("getting DisplayVersion: %w", err)
	}

	return iconPath, name, version, nil
}

func generateInstallersProgramIcons(programNames map[string]struct{}, includeIcon bool) []map[string]string {
	var installerIcons []map[string]string

	productRegPaths := map[registry.Key][]string{
		registry.CLASSES_ROOT: expandRegistryKey(registry.CLASSES_ROOT, `Installer\Products\*`),
		registry.USERS:        expandRegistryKey(registry.USERS, `*\Software\Microsoft\Installer\Products\*`),
	}

	for key, paths := range productRegPaths {
		for _, path := range paths {
			iconPath, name, err := getRegistryKeyProductData(key, path)
			if err != nil {
				continue
			}

			// If we're filtering on program names, check for that
			if len(programNames) > 0 {
				if _, found := programNames[name]; !found {
					continue
				}
			}

			icon, err := parseIcoFile(iconPath, includeIcon)
			if err != nil {
				continue
			}

			result := map[string]string{
				"hash": fmt.Sprintf("%x", icon.hash),
				"name": name,
			}

			if includeIcon {
				result["icon"] = icon.base64
			}

			installerIcons = append(installerIcons, result)
		}
	}

	return installerIcons
}

func getRegistryKeyProductData(key registry.Key, path string) (string, string, error) {
	key, err := registry.OpenKey(key, path, registry.READ)
	if err != nil {
		return "", "", fmt.Errorf("opening key: %w", err)
	}
	defer key.Close()

	iconPath, _, err := key.GetStringValue("ProductIcon")
	if err != nil {
		return "", "", fmt.Errorf("getting ProductIcon: %w", err)
	}

	name, _, err := key.GetStringValue("ProductName")
	if err != nil {
		return "", "", fmt.Errorf("getting ProductName: %w", err)
	}

	return iconPath, name, nil
}

// parseIcoFile returns a base64 encoded version and a hash of the ico.
//
// This doesn't support extracting an icon from a exe. Windows stores some icon in
// the exe like 'OneDriveSetup.exe,-101'
func parseIcoFile(fullPath string, includeIcon bool) (icon, error) {
	var programIcon icon
	expandedPath, err := registry.ExpandString(fullPath)
	if err != nil {
		return programIcon, fmt.Errorf("expanding path: %w", err)
	}
	icoReader, err := os.Open(expandedPath)
	if err != nil {
		return programIcon, fmt.Errorf("opening path: %w", err)
	}
	defer icoReader.Close()
	img, err := ico.Decode(icoReader)
	if err != nil {
		return programIcon, fmt.Errorf("decoding image: %w", err)
	}
	buf := new(bytes.Buffer)
	img = resize.Resize(128, 128, img, resize.Bilinear)
	if err := png.Encode(buf, img); err != nil {
		return programIcon, fmt.Errorf("encoding image: %w", err)
	}
	checksum := crc64.Checksum(buf.Bytes(), crcTable)
	if includeIcon {
		return icon{base64: base64.StdEncoding.EncodeToString(buf.Bytes()), hash: checksum}, nil
	}
	return icon{base64: "", hash: checksum}, nil
}

// expandRegistryKey takes a hive and path, and does a non-recursive glob expansion
//
// For example expandRegistryKey(registry.USERS, `*\Software\Microsoft\Installer\Products\*`)
// expands to
// USER1\Software\Microsoft\Installer\Products\2CCC92FB8B3D5F6499511F546A784ACD
// USER1\Software\Microsoft\Installer\Products\1AAA2FB8B3D5F6499511F546A784ACD
// USER2\Software\Microsoft\Installer\Products\3FFF92FB8B3D5F6499511F546A784ACD
// USER2\Software\Microsoft\Installer\Products\5DDD92FB8B3D5F6499511F546A784ACD
func expandRegistryKey(hive registry.Key, pattern string) []string {
	var paths []string
	magicChar := `*`

	patternsQueue := []string{pattern}
	for len(patternsQueue) > 0 {
		expandablePattern := patternsQueue[0]
		patternsQueue = patternsQueue[1:]

		// add path to results if it doesn't contain the magic char
		if !strings.Contains(expandablePattern, magicChar) {
			paths = append(paths, expandablePattern)
			continue
		}

		patternParts := strings.SplitN(expandablePattern, magicChar, 2)
		key, err := registry.OpenKey(hive, patternParts[0], registry.READ)
		if err != nil {
			continue
		}
		stats, err := key.Stat()
		if err != nil {
			continue
		}
		subKeyNames, err := key.ReadSubKeyNames(int(stats.SubKeyCount))
		if err != nil {
			continue
		}

		for _, subKeyName := range subKeyNames {
			patternsQueue = append(patternsQueue, patternParts[0]+subKeyName+patternParts[1])
		}
	}

	return paths
}
