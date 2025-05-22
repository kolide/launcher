package sleeper

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

// sleeper.Table is a debugging table, used to test queries that take a long time to run.
type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.IntegerColumn("duration"),
	}

	t := &Table{
		slogger: slogger.With("table", "kolide_deadly_sleeper"),
	}

	return tablewrapper.New(flags, slogger, "kolide_deadly_sleeper", columns, t.generate, tablewrapper.WithTableGenerateTimeout(10*time.Minute))

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, durationStr := range tablehelpers.GetConstraints(queryContext, "duration") {
		// This is using an anonymous function so that the ticker.Stop can cleanly be deferred. Elsewise, we end up
		// stacking them, and the linter complains.
		if err := func() error {
			duration, err := strconv.Atoi(durationStr)
			if err != nil {
				return err
			}

			t.slogger.Log(ctx, slog.LevelWarn, "The deadly sleeper table sleeps!", "duration", duration)

			ticker := time.NewTicker(time.Duration(duration) * time.Second)
			defer ticker.Stop()

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}

			results = append(results, map[string]string{"duration": durationStr})
			return nil
		}(); err != nil {
			return results, err
		}
	}

	return results, nil
}
