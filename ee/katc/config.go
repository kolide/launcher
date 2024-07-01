package katc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

// katcSourceType defines a source of data for a KATC table. The `name` is the
// identifier parsed from the JSON KATC config, and the `dataFunc` is the function
// that performs the query against the source.
type katcSourceType struct {
	name     string
	dataFunc func(ctx context.Context, slogger *slog.Logger, path string, query string, sourceConstraints *table.ConstraintList) ([]sourceData, error)
}

// sourceData holds the result of calling `katcSourceType.dataFunc`. It maps the
// source to the query results. (A config may have wildcards in the source,
// allowing for querying against multiple sources.)
type sourceData struct {
	path string
	rows []map[string][]byte
}

const (
	sqliteSourceType = "sqlite"
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
	default:
		return fmt.Errorf("unknown table type %s", s)
	}
}

// rowTransformStep defines an operation performed against a row of data
// returned from a source. The `name` is the identifier parsed from the
// JSON KATC config.
type rowTransformStep struct {
	name          string
	transformFunc func(ctx context.Context, slogger *slog.Logger, row map[string][]byte) (map[string][]byte, error)
}

const (
	snappyDecodeTransformStep       = "snappy"
	deserializeFirefoxTransformStep = "deserialize_firefox"
	camelToSnakeTransformStep       = "camel_to_snake"
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
	case deserializeFirefoxTransformStep:
		r.name = deserializeFirefoxTransformStep
		r.transformFunc = deserializeFirefox
		return nil
	case camelToSnakeTransformStep:
		r.name = camelToSnakeTransformStep
		r.transformFunc = camelToSnake
		return nil
	default:
		return fmt.Errorf("unknown data processing step %s", s)
	}
}

// katcTableConfig is the configuration for a specific KATC table. The control server
// sends down these configurations.
type katcTableConfig struct {
	SourceType        katcSourceType     `json:"source_type"`
	Source            string             `json:"source"` // Describes how to connect to source (e.g. path to db) -- wildcards supported
	Platform          string             `json:"platform"`
	Columns           []string           `json:"columns"`
	Query             string             `json:"query"` // Query to run against `path`
	RowTransformSteps []rowTransformStep `json:"row_transform_steps"`
}

// ConstructKATCTables takes stored configuration of KATC tables, parses the configuration,
// and returns the constructed tables.
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
