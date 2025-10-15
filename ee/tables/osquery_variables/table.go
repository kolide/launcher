package osquery_variables

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin(flags types.Flags, slogger *slog.Logger, getter types.Getter) *table.Plugin {
	columns := dataflattentable.Columns()
	return tablewrapper.New(flags, slogger, "kolide_osquery_variables", columns, generate(slogger, getter))
}

func generate(slogger *slog.Logger, getter types.Getter) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		ctx, span := observability.StartSpan(ctx, "table_name", "kolide_osquery_variables")
		defer span.End()

		results := make([]map[string]string, 0)

		variables, err := getter.Get([]byte("releases"))
		if err != nil {
			slogger.Log(ctx, slog.LevelInfo, "failure getting data from store", "err", err)
			return nil, err
		}

		flattened, err := dataflatten.Json(variables)
		if err != nil {
			slogger.Log(ctx, slog.LevelInfo, "failure flattening output", "err", err)
			return nil, err
		}

		results = append(results, dataflattentable.ToMap(flattened, "", map[string]string{})...)

		return results, nil
	}
}
