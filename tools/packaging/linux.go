package packaging

import (
	"github.com/pkg/errors"
)

// MakeLinuxPackages will create a deb and rpm package given a specific osquery
// version identifier, a munemo tenant identifier, and a key used to sign the
// enrollment secret JWT token. The output path of the package is returned and
// an error if the operation was not successful.
func MakeLinuxPackages(osqueryVersion, tenantIdentifier, hostname string, pemKey []byte) (string, string, error) {
	return "", "", errors.New("unimplemented")
}
