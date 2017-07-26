package packaging

import (
	"os/exec"

	"github.com/pkg/errors"
)

func FetchOsquerydBinary(osqueryVersion, osqueryPlatform string) (string, error) {
	if osqueryPlatform != "darwin" {
		return "", errors.New("only works locally for now until binaries are in GCS")
	}
	return exec.LookPath("osqueryd")
}
