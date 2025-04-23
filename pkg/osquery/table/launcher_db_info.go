package table

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

func LauncherDbInfo(flags types.Flags, slogger *slog.Logger, db *bbolt.DB) *table.Plugin {
	columns := dataflattentable.Columns()
	return tablewrapper.New(flags, slogger, "kolide_launcher_db_info", columns, generateLauncherDbInfo(db))
}

func generateLauncherDbInfo(db *bbolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := observability.StartSpan(ctx, "table_name", "kolide_launcher_db_info")
		defer span.End()

		stats, err := agent.GetStats(db)
		if err != nil {
			return nil, err
		}

		statsJson, err := json.Marshal(stats)
		if err != nil {
			return nil, err
		}

		var results []map[string]string

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			rows, err := dataflatten.Json(statsJson, flattenOpts...)

			if err != nil {
				// In other places, we'd log the error
				// and continue. We don't have logs
				// here, so we'll just return it.
				return results, fmt.Errorf("flattening with query %s: %w", dataQuery, err)

			}

			results = append(results, dataflattentable.ToMap(rows, dataQuery, nil)...)
		}

		return results, nil
	}
}
