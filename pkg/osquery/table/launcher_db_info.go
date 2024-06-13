package table

import (
	"context"
	"fmt"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

func LauncherDbInfo(sStatTracker types.StorageStatTracker) *table.Plugin {
	columns := dataflattentable.Columns()
	return table.NewPlugin("kolide_launcher_db_info", columns, generateLauncherDbInfo(sStatTracker))
}

func generateLauncherDbInfo(sStatTracker types.StorageStatTracker) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		if sStatTracker == nil {
			return nil, fmt.Errorf("unable to gather db info without stat tracking connection")
		}

		statsJson, err := sStatTracker.GetStats()
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
