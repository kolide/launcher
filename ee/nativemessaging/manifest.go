package nativemessaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	return writeManifest(launcherManifestFilePath(rootDir), manifestFileRegistrationLocations(hostName), hostName)
}

func launcherManifestFilePath(rootDir string) string {
	return filepath.Join(rootDir, "nmh-manifest.json")
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
func writeManifest(manifestFilePath string, registrationLocations []string, hostName string) error {
	m, err := buildManifest(hostName)
	if err != nil {
		return fmt.Errorf("building manifest: %w", err)
	}

	rawManifest, err := json.Marshal(m)
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

func buildManifest(hostName string) (*manifest, error) {
	launcherPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting current executable path: %w", err)
	}
	allowedOrigins := make([]string, 0)
	for allowedOrigin := range allowlistedDt4aOriginsLookup {
		if !strings.HasPrefix(allowedOrigin, "chrome-extension://") {
			continue
		}
		allowedOrigins = append(allowedOrigins, allowedOrigin+"/")
	}
	slices.Sort(allowedOrigins) // sort to maintain consistent ordering of origins
	return &manifest{
		Name:           hostName,
		Description:    nativeMessagingHostDescription,
		Path:           launcherPath,
		Type:           nativeMessagingInterfaceType,
		AllowedOrigins: allowedOrigins,
	}, nil
}

func RemoveNativeMessagingManifest(rootDir string, identifier string) error {
	hostName := nativeMessagingHostName(identifier)
	return removeManifest(launcherManifestFilePath(rootDir), manifestFileRegistrationLocations(hostName))
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
