package table

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/osquery/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

func LauncherInfoTable(db *bbolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("branch"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
		table.TextColumn("go_version"),
		table.TextColumn("goarch"),
		table.TextColumn("goos"),
		table.TextColumn("revision"),
		table.TextColumn("version"),
		table.TextColumn("identifier"),
		table.TextColumn("osquery_instance_id"),

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
	return table.NewPlugin("kolide_launcher_info", columns, generateLauncherInfoTable(db))
}

func generateLauncherInfoTable(db *bbolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		identifier, err := osquery.IdentifierFromDB(db)
		if err != nil {
			return nil, err
		}

		osqueryInstance, err := history.LatestInstance()
		if err != nil {
			return nil, err
		}

		publicKey, fingerprint, err := osquery.PublicRSAKeyFromDB(db)
		if err != nil {
			// No logger here, so we can't easily log. Move on with blank values
			publicKey = ""
			fingerprint = ""
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
				"identifier":          identifier,
				"osquery_instance_id": osqueryInstance.InstanceId,
				"fingerprint":         fingerprint,
				"public_key":          publicKey,
			},
		}

		// always use local key as signing key for now until k2 is updated to handle hardware keys
		var localPem bytes.Buffer
		if err := osquery.PublicKeyToPem(agent.LocalDbKeys().Public(), &localPem); err == nil {
			results[0]["signing_key"] = localPem.String()
			results[0]["signing_key_source"] = agent.LocalDbKeys().Type()
		}

		// going forward were using DER format
		localKeyDer, err := x509.MarshalPKIXPublicKey(agent.LocalDbKeys().Public())
		if err == nil {
			// der is a binary format, so convert to b64
			results[0]["local_key"] = base64.StdEncoding.EncodeToString(localKeyDer)
		}

		// we might not always have hardware keys so check first
		if agent.HardwareKeys().Public() == nil {
			return results, nil
		}

		hardwareKeyDer, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public())
		if err == nil {
			// der is a binary format, so convert to b64
			results[0]["hardware_key"] = base64.StdEncoding.EncodeToString(hardwareKeyDer)
			results[0]["hardware_key_source"] = agent.HardwareKeys().Type()
		}

		return results, nil
	}
}
