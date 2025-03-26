package table

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

func LauncherInfoTable(knapsack types.Knapsack, slogger *slog.Logger, configStore types.GetterSetter, LauncherHistoryStore types.GetterSetter) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("branch"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
		table.TextColumn("go_version"),
		table.TextColumn("goarch"),
		table.TextColumn("goos"),
		table.TextColumn("revision"),
		table.TextColumn("version"),
		table.TextColumn("version_chain"),
		table.TextColumn("registration_id"),
		table.TextColumn("identifier"),
		table.TextColumn("osquery_instance_id"),
		table.TextColumn("uptime"),

		// Signing key info
		table.TextColumn("signing_key"),
		table.TextColumn("signing_key_source"),

		// Exposure of both hardware and local keys
		table.TextColumn("local_key"),
		table.TextColumn("hardware_key"),
		table.TextColumn("hardware_key_source"),

		// Old RSA Key
		table.TextColumn("fingerprint"),
		table.TextColumn("public_key"),
	}
	return tablewrapper.New(knapsack, slogger, "kolide_launcher_info", columns, generateLauncherInfoTable(knapsack, configStore, LauncherHistoryStore))
}

func generateLauncherInfoTable(knapsack types.Knapsack, configStore types.GetterSetter, LauncherHistoryStore types.GetterSetter) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := traces.StartSpan(ctx, "table_name", "kolide_launcher_info")
		defer span.End()

		identifier, err := osquery.IdentifierFromDB(configStore, types.DefaultRegistrationID)
		if err != nil {
			return nil, err
		}

		osqueryInstanceID := ""
		osqHistory := knapsack.OsqueryHistory()
		if osqHistory != nil { // gather latest instance id if able, move on otherwise
			latestId, err := osqHistory.LatestInstanceId(types.DefaultRegistrationID)
			if err == nil {
				osqueryInstanceID = latestId
			}
		}

		uptimeBytes, err := LauncherHistoryStore.Get([]byte("process_start_time"))
		if err != nil {
			uptimeBytes = nil
		}
		uptime := "uptime not available"
		if uptimeBytes != nil {
			if startTime, err := time.Parse(time.RFC3339, string(uptimeBytes)); err == nil {
				uptime = fmt.Sprintf("%d", int64(time.Since(startTime).Seconds()))
			}
		}

		results := []map[string]string{
			{
				"branch":              version.Version().Branch,
				"build_date":          version.Version().BuildDate,
				"build_user":          version.Version().BuildUser,
				"go_version":          runtime.Version(),
				"goarch":              runtime.GOARCH,
				"goos":                runtime.GOOS,
				"revision":            version.Version().Revision,
				"version":             version.Version().Version,
				"version_chain":       os.Getenv("KOLIDE_LAUNCHER_VERSION_CHAIN"),
				"registration_id":     types.DefaultRegistrationID,
				"identifier":          identifier,
				"osquery_instance_id": osqueryInstanceID,
				"fingerprint":         "",
				"public_key":          "",
				"uptime":              uptime,
			},
		}

		// always use local key as signing key for now until k2 is updated to handle hardware keys
		var localPem bytes.Buffer
		if err := publicKeyToPem(agent.LocalDbKeys().Public(), &localPem); err == nil {
			results[0]["signing_key"] = localPem.String()
			results[0]["signing_key_source"] = agent.LocalDbKeys().Type()
		}

		// going forward were using DER format
		if localKeyDer, err := x509.MarshalPKIXPublicKey(agent.LocalDbKeys().Public()); err == nil {
			// der is a binary format, so convert to b64
			results[0]["local_key"] = base64.StdEncoding.EncodeToString(localKeyDer)
		}

		// we might not always have hardware keys so check first
		if agent.HardwareKeys().Public() == nil {
			return results, nil
		}

		if runtime.GOOS == "darwin" && agent.HardwareKeys() != nil && agent.HardwareKeys().Public() != nil {
			jsonBytes, err := json.Marshal(agent.HardwareKeys())
			if err != nil {
				return nil, fmt.Errorf("marshalling hardware keys: %w", err)
			}
			results[0]["hardware_key"] = string(jsonBytes)
			results[0]["hardware_key_source"] = agent.HardwareKeys().Type()

			return results, nil
		}

		if hardwareKeyDer, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public()); err == nil {
			// on non-darwin we'll only have 1 key for the entire machine, but we want to keep format consistent with darwin
			// so just return a map with 0 as the uid
			jsonBytes, err := json.Marshal(map[string]string{
				"0": base64.StdEncoding.EncodeToString(hardwareKeyDer),
			})

			if err != nil {
				return nil, fmt.Errorf("marshalling hardware keys: %w", err)
			}

			// der is a binary format, so convert to b64
			results[0]["hardware_key"] = string(jsonBytes)
			results[0]["hardware_key_source"] = agent.HardwareKeys().Type()
		}

		return results, nil
	}
}

func publicKeyToPem(pub any, out io.Writer) error {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("pkix marshalling: %w", err)
	}

	return pem.Encode(out, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
}
