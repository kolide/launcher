package katc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/osquery/osquery-go/plugin/table"
)

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
	for _, dataRawRow := range dataRaw {
		rowData := make(map[string]string)
		for key, val := range dataRawRow {
			// Run any processing steps on the data value
			for _, dataProcessingStep := range k.cfg.DataProcessingSteps {
				val, err = dataProcessingStep.processingFunc(ctx, val, k.slogger)
				if err != nil {
					return nil, fmt.Errorf("transforming data at key `%s`: %w", key, err)
				}
			}

			// Now transform byte => string
			rowData[key] = string(val)
		}

		results = append(results, rowData)
	}

	// Now, filter data as needed
	// TODO queryContext

	return results, nil
}
