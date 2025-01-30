package table

import (
	"context"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

func LauncherConfigTable(store types.Getter, registrationTracker types.RegistrationTracker) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("config"),
		table.TextColumn("registration_id"),
	}
	return table.NewPlugin("kolide_launcher_config", columns, generateLauncherConfig(store, registrationTracker))
}

func generateLauncherConfig(store types.Getter, registrationTracker types.RegistrationTracker) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := traces.StartSpan(ctx, "table_name", "kolide_launcher_config")
		defer span.End()

		results := make([]map[string]string, 0)
		for _, registrationId := range registrationTracker.RegistrationIDs() {
			config, err := osquery.Config(store, registrationId)
			if err != nil {
				return nil, err
			}
			results = append(results, map[string]string{
				"config":          config,
				"registration_id": registrationId,
			})
		}
		return results, nil
	}
}
