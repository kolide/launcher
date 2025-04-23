package table

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/osquery/osquery-go/plugin/table"
)

func LauncherConfigTable(flags types.Flags, slogger *slog.Logger, store types.Getter, registrationTracker types.RegistrationTracker) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("config"),
		table.TextColumn("registration_id"),
	}
	return tablewrapper.New(flags, slogger, "kolide_launcher_config", columns, generateLauncherConfig(store, registrationTracker))
}

func generateLauncherConfig(store types.Getter, registrationTracker types.RegistrationTracker) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := observability.StartSpan(ctx, "table_name", "kolide_launcher_config")
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
