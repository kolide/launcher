package nativemessaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/v2/ee/localserver"
)

// manifest represents a native messaging host config.
// See https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host
type manifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

const (
	nativeMessagingHostName        = "com.kolide.agent"
	nativeMessagingHostDescription = "Device Trust Agent"
	nativeMessagingInterfaceType   = "stdio" // This is the only possible value for "type"

	nativeMessagingHostsRegistryKeyPath = `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + nativeMessagingHostName
)

// WriteManifest is a thin wrapper around writeManifestToPaths, providing the paths
// to write the manifest to, and the registry key path for the manifest file on Windows.
func WriteManifest(rootDir string) error {
	return writeManifestToPaths(manifestFilePaths(rootDir), nativeMessagingHostsRegistryKeyPath)
}

// writeManifestToPaths builds a manifest to register launcher as a native messaging host,
// and writes it to the specified location.
func writeManifestToPaths(pathsToWrite []string, registryKeyPath string) error {
	m, err := buildManifest()
	if err != nil {
		return fmt.Errorf("building manifest: %w", err)
	}

	rawManifest, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}

	for _, pathToWrite := range pathsToWrite {
		// Check first to make sure the directory we want to write to exists -- if it doesn't,
		// we assume that Chrome/Chrome for Testing/Chromium isn't installed, and skip it.
		// This check is only relevant for macOS and Linux; on Windows, we're writing to the
		// launcher root directory instead, which is guaranteed to exist at this point.
		if _, err := os.Stat(filepath.Dir(pathToWrite)); err != nil && os.IsNotExist(err) {
			continue
		}

		if err := os.WriteFile(pathToWrite, rawManifest, 0755); err != nil {
			return fmt.Errorf("writing manifest to %s: %w", pathToWrite, err)
		}

		if err := registerManifestFileLocation(pathToWrite, registryKeyPath); err != nil {
			return fmt.Errorf("registering manifest file location at %s: %w", pathToWrite, err)
		}
	}

	return nil
}

func buildManifest() (*manifest, error) {
	launcherPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting current executable path: %w", err)
	}
	allowedOrigins := make([]string, 0)
	for allowedOrigin := range localserver.AllowlistedDt4aOriginsLookup {
		if !strings.HasPrefix(allowedOrigin, "chrome-extension") {
			continue
		}
		allowedOrigins = append(allowedOrigins, allowedOrigin)
	}
	return &manifest{
		Name:           nativeMessagingHostName,
		Description:    nativeMessagingHostDescription,
		Path:           launcherPath,
		Type:           nativeMessagingInterfaceType,
		AllowedOrigins: allowedOrigins,
	}, nil
}
