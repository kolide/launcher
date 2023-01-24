package table

import (
	"bytes"
	"context"
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

		// New hardware and local keys
		table.TextColumn("signing_key"),
		table.TextColumn("signing_key_source"),
		table.TextColumn("local_key"),

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

		// No logger, so just ignore errors. generate the pem encoding if we can.
		if eccKey := agent.Keys.Public(); eccKey != nil {
			var pem bytes.Buffer
			if err := osquery.PublicKeyToPem(eccKey, &pem); err == nil {
				results[0]["signing_key"] = pem.String()
				results[0]["signing_key_source"] = "TBD"
			}
		}

		if localKey := agent.LocalDbKeys.Public(); localKey != nil {
			var pem bytes.Buffer
			if err := osquery.PublicKeyToPem(localKey, &pem); err == nil {
				results[0]["local_key"] = pem.String()
			}
		}

		return results, nil
	}
}
