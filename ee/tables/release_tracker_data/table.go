package release_tracker_data

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/dataflattentable"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger   *slog.Logger
	tableName string
}

func TablePlugin(flags types.Flags, slogger *slog.Logger, store types.KVStore) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		slogger:   slogger.With("table", "kolide_sourced_data"),
		tableName: "kolide_sourced_data",
	}

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generateKolideReleaseTrackerDataTable(store),
		tablewrapper.WithDescription("Third party release metadata from Kolide's release tracker, flattened as key-value pairs. Useful for inspecting available software release information."),
		tablewrapper.WithNote(dataflattentable.EAVNote),
	)
}

func (t *Table) generateKolideReleaseTrackerDataTable(store types.KVStore) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
		defer span.End()

		results := make([]map[string]string, 0)

		prefilter, err := dataflattentable.ExtractPrefilterFromQuery(queryContext)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelWarn,
				"could not extract prefilter",
				"err", err,
			)
			return nil, fmt.Errorf("extracting prefilter from query context: %w", err)
		}

		data, err := store.Get([]byte("releases"))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo, "failure getting data from store", "err", err)
			return nil, err
		}

		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithSlogger(t.slogger),
		}
		flattenOpts = append(flattenOpts, prefilter.Opts()...) // no-op if the prefilter doesn't exist

		flattened, err := dataflatten.Json(data, flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo, "failure flattening output", "err", err)
			return nil, err
		}

		results = append(results, dataflattentable.ToMap(flattened, "", prefilter.Expr(), map[string]string{})...)

		return results, nil
	}
}
