package osquery

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

func getEnrollDetails(client Querier) (service.EnrollmentDetails, error) {
	var details service.EnrollmentDetails

	// To facilitate manual testing around missing enrollment details,
	// there is a environmental variable to trigger the failure condition
	if os.Getenv("LAUNCHER_DEBUG_ENROLL_DETAILS_ERROR") == "true" {
		return details, errors.New("Skipping enrollment details")
	}

	// This condition is indicative of a misordering (or race) in
	// startup. Enrollment has started before `SetQuerier` has
	// been called.
	if client == nil {
		return details, errors.New("no querier")
	}

	query := `
	SELECT
		osquery_info.version as osquery_version,
		os_version.build as os_build,
		os_version.name as os_name,
		os_version.platform as os_platform,
		os_version.platform_like as os_platform_like,
		os_version.version as os_version,
		system_info.hardware_model,
		system_info.hardware_serial,
		system_info.hardware_vendor,
		system_info.hostname,
		system_info.uuid as hardware_uuid
	FROM
		os_version,
		system_info,
		osquery_info;
`
	resp, err := client.Query(query)
	if err != nil {
		return details, fmt.Errorf("query enrollment details: %w", err)
	}

	if len(resp) < 1 {
		return details, errors.New("expected at least one row from the enrollment details query")
	}

	if val, ok := resp[0]["os_version"]; ok {
		details.OSVersion = val
	}
	if val, ok := resp[0]["os_build"]; ok {
		details.OSBuildID = val
	}
	if val, ok := resp[0]["os_name"]; ok {
		details.OSName = val
	}
	if val, ok := resp[0]["os_platform"]; ok {
		details.OSPlatform = val
	}
	if val, ok := resp[0]["os_platform_like"]; ok {
		details.OSPlatformLike = val
	}
	if val, ok := resp[0]["osquery_version"]; ok {
		details.OsqueryVersion = val
	}
	if val, ok := resp[0]["hardware_model"]; ok {
		details.HardwareModel = val
	}
	details.HardwareSerial = serialForRow(resp[0])
	if val, ok := resp[0]["hardware_vendor"]; ok {
		details.HardwareVendor = val
	}
	if val, ok := resp[0]["hostname"]; ok {
		details.Hostname = val
	}
	if val, ok := resp[0]["hardware_uuid"]; ok {
		details.HardwareUUID = val
	}

	// This runs before the extensions are registered. These mirror the
	// underlying tables.
	details.LauncherVersion = version.Version().Version
	details.GOOS = runtime.GOOS
	details.GOARCH = runtime.GOARCH

	// Pull in some launcher key info. These depend on the agent package, and we'll need to check for nils
	if agent.LocalDbKeys().Public() != nil {
		if key, err := x509.MarshalPKIXPublicKey(agent.LocalDbKeys().Public()); err == nil {
			// der is a binary format, so convert to b64
			details.LauncherLocalKey = base64.StdEncoding.EncodeToString(key)
		}
	}
	if agent.HardwareKeys().Public() != nil {
		if key, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public()); err == nil {
			// der is a binary format, so convert to b64
			details.LauncherHardwareKey = base64.StdEncoding.EncodeToString(key)
			details.LauncherHardwareKeySource = agent.HardwareKeys().Type()
		}
	}

	return details, nil
}
