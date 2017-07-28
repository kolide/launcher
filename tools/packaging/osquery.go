package packaging

import (
	"os/exec"

	"github.com/pkg/errors"
)

// FetchOsquerydBinary is a stub at the moment. The following will be true when
// this method is properly implemented:
//
// FetchOsquerydBinary will synchronously download an osquery binary as per the
// supplied desired osquery version and platform identifiers. The path to the
// downloaded binary is returned and an error if the operation did not succeed.
func FetchOsquerydBinary(osqueryVersion, osqueryPlatform string) (string, error) {
	if osqueryPlatform != "darwin" {
		return "", errors.New("only works locally for now until binaries are in GCS")
	}
	return exec.LookPath("osqueryd")
}
