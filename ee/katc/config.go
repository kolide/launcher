package katc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

/*
Open qs:
- Should we go with the EAV approach rather than with columns? Look at how dataflatten does it

TODOs:
- Need to do queryContext filtering
*/

type katcSourceType struct {
	name     string
	dataFunc func(ctx context.Context, slogger *slog.Logger, path string, query string) ([]sourceData, error)
}

type sourceData struct {
	path string
	rows []map[string][]byte
}

const (
	sqliteSourceType    = "sqlite"
	indexedDBSourceType = "indexeddb"
)

func (kst *katcSourceType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("unmarshalling string: %w", err)
	}

	switch s {
	case sqliteSourceType:
		kst.name = sqliteSourceType
		kst.dataFunc = sqliteData
		return nil
	case indexedDBSourceType:
		kst.name = indexedDBSourceType
		return errors.New("indexeddb is not yet implemented")
	default:
		return fmt.Errorf("unknown table type %s", s)
	}
}

type dataProcessingStep struct {
	name           string
	processingFunc func(ctx context.Context, slogger *slog.Logger, data []byte) ([]byte, error)
}

const (
	snappyDecodeProcessingStep               = "snappy"
	structuredCloneDeserializeProcessingStep = "structured_clone"
)

func (d *dataProcessingStep) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("unmarshalling string: %w", err)
	}

	switch s {
	case snappyDecodeProcessingStep:
		d.name = snappyDecodeProcessingStep
		d.processingFunc = snappyDecode
		return nil
	case structuredCloneDeserializeProcessingStep:
		d.name = structuredCloneDeserializeProcessingStep
		d.processingFunc = structuredCloneDeserialize
		return nil
	default:
		return fmt.Errorf("unknown data processing step %s", s)
	}
}

type katcTableConfig struct {
	Source              katcSourceType       `json:"source"`
	Platform            string               `json:"platform"`
	Columns             []string             `json:"columns"`
	Path                string               `json:"path"`  // Path to file holding data (e.g. sqlite file) -- wildcards supported
	Query               string               `json:"query"` // Query to run against `path`
	DataProcessingSteps []dataProcessingStep `json:"data_processing_steps"`
}

func ConstructKATCTables(config map[string]string, slogger *slog.Logger) []osquery.OsqueryPlugin {
	plugins := make([]osquery.OsqueryPlugin, 0)
	for tableName, tableConfigStr := range config {
		var cfg katcTableConfig
		if err := json.Unmarshal([]byte(tableConfigStr), &cfg); err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"unable to unmarshal config for Kolide ATC table, skipping",
				"table_name", tableName,
				"err", err,
			)
			continue
		}

		if cfg.Platform != runtime.GOOS {
			continue
		}

		columns := []table.ColumnDefinition{
			{
				Name: sourcePathColumnName,
				Type: table.ColumnTypeText,
			},
		}
		for i := 0; i < len(cfg.Columns); i += 1 {
			columns = append(columns, table.ColumnDefinition{
				Name: cfg.Columns[i],
				Type: table.ColumnTypeText,
			})
		}

		t := newKatcTable(tableName, cfg, slogger)
		plugins = append(plugins, table.NewPlugin(tableName, columns, t.generate))
	}

	return plugins
}
