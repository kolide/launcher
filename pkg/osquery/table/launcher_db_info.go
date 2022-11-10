package table

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"

	"go.etcd.io/bbolt"
)

func LauncherDbInfo(db *bbolt.DB) *table.Plugin {
	columns := dataflattentable.Columns()
	return table.NewPlugin("kolide_launcher_db_info", columns, generateLauncherDbInfo(db))
}

func generateLauncherDbInfo(db *bbolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
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
