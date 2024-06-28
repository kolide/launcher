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

type katcSourceType struct {
	name     string
	dataFunc func(ctx context.Context, slogger *slog.Logger, path string, query string, sourceConstraints *table.ConstraintList) ([]sourceData, error)
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

type rowTransformStep struct {
	name          string
	transformFunc func(ctx context.Context, slogger *slog.Logger, row map[string][]byte) (map[string][]byte, error)
}

const (
	snappyDecodeTransformStep               = "snappy"
	structuredCloneDeserializeTransformStep = "structured_clone"
)

func (r *rowTransformStep) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("unmarshalling string: %w", err)
	}

	switch s {
	case snappyDecodeTransformStep:
		r.name = snappyDecodeTransformStep
		r.transformFunc = snappyDecode
		return nil
	case structuredCloneDeserializeTransformStep:
		r.name = structuredCloneDeserializeTransformStep
		r.transformFunc = structuredCloneDeserialize
		return nil
	default:
		return fmt.Errorf("unknown data processing step %s", s)
	}
}

type katcTableConfig struct {
	Source            katcSourceType     `json:"source"`
	Platform          string             `json:"platform"`
	Columns           []string           `json:"columns"`
	Path              string             `json:"path"`  // Path to file holding data (e.g. sqlite file) -- wildcards supported
	Query             string             `json:"query"` // Query to run against `path`
	RowTransformSteps []rowTransformStep `json:"row_transform_steps"`
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

		t, columns := newKatcTable(tableName, cfg, slogger)
		plugins = append(plugins, table.NewPlugin(tableName, columns, t.generate))
	}

	return plugins
}
