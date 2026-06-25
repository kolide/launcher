package nativemessaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type (
	// See https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host
	chromeManifest struct {
		manifest
		AllowedOrigins []string `json:"allowed_origins"` // Chrome-only
	}
	// See https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_manifests
	firefoxManifest struct {
		manifest
		AllowedExtensions []string `json:"allowed_extensions"` // Firefox-only
	}
	// manifest represents a native messaging host config -- configs between browsers largely have overlap,
	// but Firefox will not load a config that has `allowed_origins` in it, so we have to keep
	// chromeManifest and firefoxManifest separate.
	manifest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Type        string `json:"type"`
	}
)

const (
	nativeMessagingHostDescription = "Device Trust Agent"
	nativeMessagingInterfaceType   = "stdio" // This is the only possible value for "type"
)

var (
	// allowlistedDt4aOriginsLookup contains the complete list of origins that are permitted to talk to launcher,
	// via the /dt4a endpoint in localserver and via native messaging here.
	allowlistedDt4aOriginsLookup = map[string]struct{}{
		// Release extension
		"chrome-extension://gejiddohjgogedgjnonbofjigllpkmbf":  {},
		"chrome-extension://khgocmkkpikpnmmkgmdnfckapcdkgfaf":  {},
		"chrome-extension://aeblfdkhhhdcdjpifhhbdiojplfjncoa":  {},
		"chrome-extension://dppgmdbiimibapkepcbdbmkaabgiofem":  {},
		"moz-extension://dfbae458-fb6f-4614-856e-094108a80852": {},
		"moz-extension://25fc87fa-4d31-4fee-b5c1-c32a7844c063": {},
		"moz-extension://d634138d-c276-4fc8-924b-40a0ea21d284": {},
		// Development and internal builds
		"chrome-extension://hjlinigoblmkhjejkmbegnoaljkphmgo":  {},
		"moz-extension://0a75d802-9aed-41e7-8daa-24c067386e82": {},
		"chrome-extension://hiajhnnfoihkhlmfejoljaokdpgboiea":  {},
		"chrome-extension://kioanpobaefjdloichnjebbdafiloboa":  {},
		"chrome-extension://bkpbhnjcbehoklfkljkkbbmipaphipgl":  {},
		// Development web app
		"https://my.b5local.com:4000":           {},
		"https://dev.sites.gitlab.1password.io": {},
	}
)

// AllowlistedDt4aOriginsLookup exports allowlistedDt4aOriginsLookup for use
// elsewhere, e.g. in the localserver package. Returning the underlying map
// technically would allow a caller outside this package to modify it -- we trust
// callers to not do that, and we prefer to not return `maps.Copy(allowlistedDt4aOriginsLookup)`
// to avoid e.g. a map copy operation on every single request to the dt4a endpoint.
func AllowlistedDt4aOriginsLookup() map[string]struct{} {
	return allowlistedDt4aOriginsLookup
}

// WriteNativeMessagingManifest is a thin wrapper around writeManifest, providing the paths
// to write the manifest to, and the registry key path for the manifest file on Windows.
func WriteNativeMessagingManifest(rootDir string, identifier string) error {
	hostName := nativeMessagingHostName(identifier)
	chromeManifest, firefoxManifest, err := buildManifests(hostName)
	if err != nil {
		return fmt.Errorf("building manifests: %w", err)
	}

	chromeWriteErr := writeManifest(chromeManifest, launcherChromeManifestFilePath(rootDir), chromeManifestFileRegistrationLocations(hostName))
	firefoxWriteErr := writeManifest(firefoxManifest, launcherFirefoxManifestFilePath(rootDir), firefoxManifestFileRegistrationLocations(hostName))
	if chromeWriteErr != nil || firefoxWriteErr != nil {
		return fmt.Errorf("writing manifest files: chrome: %v; firefox: %v", chromeWriteErr, firefoxWriteErr)
	}
	return nil
}

func launcherChromeManifestFilePath(rootDir string) string {
	return filepath.Join(rootDir, "chrome-nmh-manifest.json")
}

func launcherFirefoxManifestFilePath(rootDir string) string {
	return filepath.Join(rootDir, "firefox-nmh-manifest.json")
}

func nativeMessagingHostName(identifier string) string {
	hostName := fmt.Sprintf("com.%s.agent", identifier)
	// Identifiers typically contain hyphens, but only lowercase alphanumeric characters, underscores,
	// and periods are permitted.
	return strings.ReplaceAll(hostName, "-", "_")
}

// writeManifest builds a manifest to register launcher as a native messaging host,
// and writes it to the specified location at `manifestFilePath`, and then registers that location
// with each location in `registrationLocations`.
func writeManifest(manifestToWrite any, manifestFilePath string, registrationLocations []string) error {
	rawManifest, err := json.Marshal(manifestToWrite)
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}

	// Write the manifest file
	if err := os.WriteFile(manifestFilePath, rawManifest, 0644); err != nil {
		return fmt.Errorf("writing manifest to %s: %w", manifestFilePath, err)
	}

	// Now, register `manifestFilePath` with each of our registration locations.
	// On macOS and Linux, we do this by creating symlinks at well-known locations
	// to `manifestFilePath`; on Windows, we do this by writing `manifestFilePath`
	// to a well-known registry key.
	registrationErrs := make([]error, 0)
	for _, registrationLocation := range registrationLocations {
		if err := registerManifestFileLocation(manifestFilePath, registrationLocation); err != nil {
			registrationErrs = append(registrationErrs, fmt.Errorf("registering manifest file location at %s: %w", registrationLocation, err))
		}
	}

	if len(registrationErrs) > 0 {
		return fmt.Errorf("registering manifest with one or more locations: %+v: %w", registrationErrs, registrationErrs[0])
	}

	return nil
}

func buildManifests(hostName string) (*chromeManifest, *firefoxManifest, error) {
	launcherPath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current executable path: %w", err)
	}
	sharedManifest := manifest{
		Name:        hostName,
		Description: nativeMessagingHostDescription,
		Path:        launcherPath,
		Type:        nativeMessagingInterfaceType,
	}

	allowedChromeOrigins := make([]string, 0)
	allowedFirefoxExtensions := make([]string, 0)
	for allowedOrigin := range allowlistedDt4aOriginsLookup {
		if strings.HasPrefix(allowedOrigin, "chrome-extension://") {
			allowedChromeOrigins = append(allowedChromeOrigins, allowedOrigin+"/")
			continue
		}
		if originWithPrefixTrimmed, ok := strings.CutPrefix(allowedOrigin, "moz-extension://"); ok {
			allowedFirefoxExtensions = append(allowedFirefoxExtensions, fmt.Sprintf("{%s}", originWithPrefixTrimmed))
			continue
		}
	}

	return &chromeManifest{
			manifest:       sharedManifest,
			AllowedOrigins: allowedChromeOrigins,
		}, &firefoxManifest{
			manifest:          sharedManifest,
			AllowedExtensions: allowedFirefoxExtensions,
		}, nil
}

func RemoveNativeMessagingManifest(rootDir string, identifier string) error {
	hostName := nativeMessagingHostName(identifier)
	return removeManifest(launcherChromeManifestFilePath(rootDir), chromeManifestFileRegistrationLocations(hostName))
}

func removeManifest(manifestFilePath string, registrationLocations []string) error {
	deregistrationErrs := make([]error, 0)
	// First, delete the registration locations
	for _, registrationLocation := range registrationLocations {
		if err := deregisterManifestFileLocation(registrationLocation); err != nil {
			deregistrationErrs = append(deregistrationErrs, fmt.Errorf("deregistering manifest at %s: %w", registrationLocation, err))
		}
	}

	if len(deregistrationErrs) > 0 {
		return fmt.Errorf("deregistering manifest with one or more locations: %+v: %w", deregistrationErrs, deregistrationErrs[0])
	}

	// Finally, remove the manifest file
	if err := os.Remove(manifestFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", manifestFilePath, err)
	}

	return nil
}
