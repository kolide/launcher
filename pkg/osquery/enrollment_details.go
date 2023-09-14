package osquery

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

func getEnrollDetails(ctx context.Context, osquerydPath string) (service.EnrollmentDetails, error) {
	var details service.EnrollmentDetails

	// To facilitate manual testing around missing enrollment details,
	// there is a environmental variable to trigger the failure condition
	if os.Getenv("LAUNCHER_DEBUG_ENROLL_DETAILS_ERROR") == "true" {
		return details, errors.New("Skipping enrollment details")
	}

	// If the binary doesn't exist, bail out early.
	if info, err := os.Stat(osquerydPath); os.IsNotExist(err) {
		return details, fmt.Errorf("no binary at %s", osquerydPath)
	} else if info.IsDir() {
		return details, fmt.Errorf("%s is a directory", osquerydPath)
	} else if err != nil {
		return details, fmt.Errorf("statting %s: %w", osquerydPath, err)
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

	var respBytes bytes.Buffer

	osq, err := runsimple.NewOsqueryProcess(
		osquerydPath,
		runsimple.RunSql([]byte(query)),
		runsimple.WithStdout(&respBytes),
	)
	if err != nil {
		return details, fmt.Errorf("create osquery for enrollment details: %w", err)
	}

	osqCtx, osqCancel := context.WithTimeout(ctx, 5*time.Second)
	defer osqCancel()

	if err := osq.Execute(osqCtx); osqCtx.Err() != nil {
		return details, fmt.Errorf("query enrollment details context error: %w", osqCtx.Err())
	} else if err != nil {
		return details, fmt.Errorf("query enrollment details: %w", err)
	}

	var resp []map[string]string
	if err := json.Unmarshal(respBytes.Bytes(), &resp); err != nil {
		return details, fmt.Errorf("json decode enrollment details: %w", err)
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
