package packaging

import (
	"fmt"
	"os/exec"
)

// FetchOsquerydBinary will synchronously download an osquery binary as per the
// supplied desired osquery version and platform identifiers. The path to the
// downloaded binary is returned and an error if the operation did not succeed.
func FetchOsquerydBinary(osqueryVersion, osqueryPlatform string) (string, error) {
	// Check that the arguments are valid

	// See if a local package exists on disk already

	// If so, return the cached path

	// If not we have to download the package

	// Create download URI
	url := fmt.Sprintf("https://dl.kolide.com/kolide/osqueryd/%s/osqueryd-%s.tar.gz", osqueryPlatform, osqueryVersion)
	_ = url

	// Download the package

	// Store it in cache

	// Return the cached path of the downloaded package

	return exec.LookPath("osqueryd")
}
