package katc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/osquery/osquery-go/plugin/table"
)

const sourcePathColumnName = "source_path"

type katcTable struct {
	cfg     katcTableConfig
	slogger *slog.Logger
}

func newKatcTable(tableName string, cfg katcTableConfig, slogger *slog.Logger) *katcTable {
	return &katcTable{
		cfg: cfg,
		slogger: slogger.With(
			"table_name", tableName,
			"table_type", cfg.Source,
			"table_path", cfg.Path,
		),
	}
}

func (k *katcTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	// Fetch data from our table source
	dataRaw, err := k.cfg.Source.dataFunc(ctx, k.slogger, k.cfg.Path, k.cfg.Query)
	if err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	// Process data
	results := make([]map[string]string, 0)
	for _, s := range dataRaw {
		for _, dataRawRow := range s.rows {
			// Make sure source is included in row data
			rowData := map[string]string{
				sourcePathColumnName: s.path,
			}

			// Run any needed transformations on the row data
			for _, step := range k.cfg.RowTransformSteps {
				dataRawRow, err = step.transformFunc(ctx, k.slogger, dataRawRow)
				if err != nil {
					return nil, fmt.Errorf("running transform func %s: %w", step.name, err)
				}
			}

			// After transformations have been applied, we can cast the data from []byte
			// to string to return to osquery.
			for key, val := range dataRawRow {
				rowData[key] = string(val)
			}
			results = append(results, rowData)
		}
	}

	// Now, filter data as needed
	// TODO queryContext

	return results, nil
}
