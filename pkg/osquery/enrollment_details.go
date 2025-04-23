package osquery

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

// getEnrollDetails returns an EnrollmentDetails struct with populated with details it can fetch without osquery.
// To get the rest of the details, pass the struct to getOsqEnrollDetails.
func getRuntimeEnrollDetails() service.EnrollmentDetails {
	details := service.EnrollmentDetails{
		OSPlatform:      runtime.GOOS,
		OSPlatformLike:  runtime.GOOS,
		LauncherVersion: version.Version().Version,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
	}

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

	return details
}

// getOsqEnrollDetails queries osquery for enrollment details and populates the EnrollmentDetails struct.
// It's expected that the caller has initially populated the struct with runtimeEnrollDetails by calling getRuntimeEnrollDetails.
func getOsqEnrollDetails(ctx context.Context, osquerydPath string, details *service.EnrollmentDetails) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// To facilitate manual testing around missing enrollment details,
	// there is a environmental variable to trigger the failure condition
	if os.Getenv("LAUNCHER_DEBUG_ENROLL_DETAILS_ERROR") == "true" {
		return errors.New("Skipping enrollment details")
	}

	// If the binary doesn't exist, bail out early.
	if info, err := os.Stat(osquerydPath); os.IsNotExist(err) {
		return fmt.Errorf("no binary at %s", osquerydPath)
	} else if info.IsDir() {
		return fmt.Errorf("%s is a directory", osquerydPath)
	} else if err != nil {
		return fmt.Errorf("statting %s: %w", osquerydPath, err)
	}

	query := `
	SELECT
		osquery_info.version as osquery_version,
		os_version.build as os_build,
		os_version.name as os_name,
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
	var stderrBytes bytes.Buffer

	osq, err := runsimple.NewOsqueryProcess(
		osquerydPath,
		runsimple.WithStdout(&respBytes),
		runsimple.WithStderr(&stderrBytes),
	)
	if err != nil {
		return fmt.Errorf("create osquery for enrollment details: %w", err)
	}

	osqCtx, osqCancel := context.WithTimeout(ctx, 5*time.Second)
	defer osqCancel()

	if sqlErr := osq.RunSql(osqCtx, []byte(query)); osqCtx.Err() != nil {
		return fmt.Errorf("query enrollment details context error: %w: stderr: %s", osqCtx.Err(), stderrBytes.String())
	} else if sqlErr != nil {
		return fmt.Errorf("query enrollment details: %w; stderr: %s", sqlErr, stderrBytes.String())
	}

	var resp []map[string]string
	if err := json.Unmarshal(respBytes.Bytes(), &resp); err != nil {
		return fmt.Errorf("json decode enrollment details: %w; stderr: %s", err, stderrBytes.String())
	}

	if len(resp) < 1 {
		return fmt.Errorf("expected at least one row from the enrollment details query: stderr: %s", stderrBytes.String())
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

	return nil
}

// CollectAndSetEnrollmentDetails collects enrollment details from osquery and sets them in the knapsack.
func CollectAndSetEnrollmentDetails(ctx context.Context, slogger *slog.Logger, k types.Knapsack, collectTimeout time.Duration, collectRetryInterval time.Duration) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// Get the runtime details
	details := getRuntimeEnrollDetails()

	// Set the osquery version and save everything to knapsack before attempting to get osquery enrollment details
	k.SetEnrollmentDetails(details)

	latestOsquerydPath := k.LatestOsquerydPath(ctx)

	if latestOsquerydPath == "" {
		slogger.Log(ctx, slog.LevelWarn,
			"osqueryd path is empty, cannot collect enrollment details from osquery",
		)
		return
	}

	if err := backoff.WaitFor(func() error {
		err := getOsqEnrollDetails(ctx, latestOsquerydPath, &details)
		if err != nil {
			span.AddEvent("failed to get enrollment details")
			slogger.Log(ctx, slog.LevelDebug,
				"failed to get enrollment details",
				"osqueryd_path", latestOsquerydPath,
				"err", err,
			)
		}
		return err
	}, collectTimeout, collectRetryInterval); err != nil {
		observability.SetError(span, fmt.Errorf("enrollment details collection failed: %w", err))
		slogger.Log(ctx, slog.LevelWarn,
			"could not fetch osqueryd enrollment details before timeout",
			"osqueryd_path", latestOsquerydPath,
			"err", err,
		)
	}

	k.SetEnrollmentDetails(details)
}
