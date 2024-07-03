package katc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

// katcSourceType defines a source of data for a KATC table. The `name` is the
// identifier parsed from the JSON KATC config, and the `dataFunc` is the function
// that performs the query against the source.
type katcSourceType struct {
	name     string
	dataFunc func(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, pathConstraints *table.ConstraintList) ([]sourceData, error)
}

// sourceData holds the result of calling `katcSourceType.dataFunc`. It maps the
// source to the query results. (A config may have wildcards in the source,
// allowing for querying against multiple sources.)
type sourceData struct {
	path string
	rows []map[string][]byte
}

const (
	sqliteSourceType           = "sqlite"
	indexeddbLeveldbSourceType = "indexeddb_leveldb"
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
	case indexeddbLeveldbSourceType:
		kst.name = indexeddbLeveldbSourceType
		kst.dataFunc = indexeddbLeveldbData
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
	deserializeChromeTransformStep  = "deserialize_chrome"
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
	case deserializeChromeTransformStep:
		r.name = deserializeChromeTransformStep
		r.transformFunc = indexeddb.DeserializeChrome
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
	Name              string             `json:"name"`
	SourceType        katcSourceType     `json:"source_type"`
	SourcePaths       []string           `json:"source_paths"` // Describes how to connect to source (e.g. path to db) -- % and _ wildcards supported
	Filter            string             `json:"filter"`
	Columns           []string           `json:"columns"`
	SourceQuery       string             `json:"source_query"` // Query to run against each source path
	RowTransformSteps []rowTransformStep `json:"row_transform_steps"`
}

// ConstructKATCTables takes stored configuration of KATC tables, parses the configuration,
// and returns the constructed tables.
func ConstructKATCTables(config map[string]string, slogger *slog.Logger) []osquery.OsqueryPlugin {
	plugins := make([]osquery.OsqueryPlugin, 0)

	tableConfigs, tableConfigsExist := config["tables"]
	if !tableConfigsExist {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"missing top-level tables key in KATC config, cannot construct tables",
		)

		return plugins
	}

	// We want to unmarshal each table config separately, so that we don't fail to configure all tables
	// if only some are malformed.
	var rawTableConfigs []json.RawMessage
	if err := json.Unmarshal([]byte(tableConfigs), &rawTableConfigs); err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"could not unmarshal tables in KATC config",
			"err", err,
		)
		return plugins
	}

	for _, rawTableConfig := range rawTableConfigs {
		var cfg katcTableConfig
		if err := json.Unmarshal(rawTableConfig, &cfg); err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"unable to unmarshal config for KATC table, skipping",
				"err", err,
			)
			continue
		}

		// For now, the filter is simply the OS
		if cfg.Filter != runtime.GOOS {
			continue
		}

		t, columns := newKatcTable(cfg, slogger)
		plugins = append(plugins, table.NewPlugin(cfg.Name, columns, t.generate))
	}

	return plugins
}
