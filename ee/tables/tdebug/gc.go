package tdebug

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	gcTableName = "launcher_gc_info"
)

type gcTable struct {
	slogger *slog.Logger
	stats   debug.GCStats
}

func LauncherGcInfo(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &gcTable{
		slogger: slogger.With("table", gcTableName),
	}

	return tablewrapper.New(flags, slogger, gcTableName, columns, t.generate)
}

func (t *gcTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", gcTableName)
	defer span.End()

	var results []map[string]string

	debug.ReadGCStats(&t.stats)

	// Make sure the history arrays aren't too large
	if len(t.stats.Pause) > 100 {
		t.stats.Pause = t.stats.Pause[:100]
	}
	if len(t.stats.PauseEnd) > 100 {
		t.stats.PauseEnd = t.stats.PauseEnd[:100]
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		// bounce through json to serialize GCStats
		jsonBytes, err := json.Marshal(t.stats)
		if err != nil {
			return nil, fmt.Errorf("json: %w", err)
		}

		flatData, err := dataflatten.Json(
			jsonBytes,
			dataflatten.WithSlogger(t.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"gc flatten failed",
				"err", err,
			)
			continue
		}
		results = append(results, dataflattentable.ToMap(flatData, dataQuery, nil)...)
	}
	return results, nil
}
