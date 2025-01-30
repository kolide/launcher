package tablewrapper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const DefaultTableTimeout = 2 * time.Minute

type generateResult struct {
	rows []map[string]string
	err  error
}

// NewTablePluginWithTimeout returns a table plugin using the default table timeout.
// Most tables should use this function unless they have a specific need for a
// custom timeout.
func NewTablePluginWithTimeout(slogger *slog.Logger, name string, columns []table.ColumnDefinition, gen table.GenerateFunc) *table.Plugin {
	return NewTablePluginWithCustomTimeout(slogger, name, columns, gen, DefaultTableTimeout)
}

// NewTablePluginWithCustomTimeout returns a table plugin that will attempt to execute a query
// up until the given timeout, at which point it will instead return no rows and a timeout error.
func NewTablePluginWithCustomTimeout(slogger *slog.Logger, name string, columns []table.ColumnDefinition, gen table.GenerateFunc, genTimeout time.Duration) *table.Plugin {
	wrappedGen := func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		ctx, span := traces.StartSpan(ctx, "table_name", name)
		defer span.End()

		ctx, cancel := context.WithTimeout(ctx, genTimeout)
		defer cancel()

		// Kick off running the query
		resultChan := make(chan *generateResult)
		gowrapper.Go(ctx, slogger, func() {
			rows, err := gen(ctx, queryContext)
			span.AddEvent("generate_returned")
			resultChan <- &generateResult{
				rows: rows,
				err:  err,
			}
		})

		// Wait for results up until the timeout
		select {
		case result := <-resultChan:
			return result.rows, result.err
		case <-ctx.Done():
			queriedColumns := columnsFromConstraints(queryContext)
			slogger.Log(ctx, slog.LevelWarn,
				"query timed out",
				"table_name", name,
				"queried_columns", fmt.Sprintf("%+v", queriedColumns),
			)
			return nil, fmt.Errorf("querying %s timed out after %s (queried columns: %v)", name, genTimeout.String(), queriedColumns)
		}
	}

	return table.NewPlugin(name, columns, wrappedGen)
}

// columnsFromConstraints extracts the column names from the query for logging purposes
func columnsFromConstraints(queryContext table.QueryContext) []string {
	keys := make([]string, 0, len(queryContext.Constraints))
	for k := range queryContext.Constraints {
		keys = append(keys, k)
	}

	return keys
}
