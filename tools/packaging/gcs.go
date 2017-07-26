package packaging

import (
	"github.com/pkg/errors"
)

// UploadMacOSPkgToGCS takes a package at a specified path and uploads it to GCS
// so that it can be downloaded and used by a client specified by the tenant
// identifier for the specified hostname.
func UploadMacOSPkgToGCS(macPackagePath, tenantIdentifier, hostname string) error {
	return errors.New("not implemented")
}
