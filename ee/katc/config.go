package katc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

// katcSourceType defines a source of data for a KATC table. The `name` is the
// identifier parsed from the JSON KATC config, and the `dataFunc` is the function
// that performs the query against the source.
type katcSourceType struct {
	name string
	// queryContext contains the constraints from the WHERE clause of the query against the KATC table.
	dataFunc func(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, queryContext table.QueryContext) ([]sourceData, error)
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
	leveldbSourceType          = "leveldb"
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
	case leveldbSourceType:
		kst.name = leveldbSourceType
		kst.dataFunc = leveldbData
		return nil
	default:
		return fmt.Errorf("unknown table type %s", s)
	}
}

func (kst *katcSourceType) String() string {
	if kst == nil {
		return ""
	}
	return kst.name
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
	hexDecodeTransformStep          = "hex"
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
	case hexDecodeTransformStep:
		r.name = hexDecodeTransformStep
		r.transformFunc = hexDecode
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

type (
	// katcTableConfig is the configuration for a specific KATC table. The control server
	// sends down these configurations.
	katcTableConfig struct {
		Columns []string `json:"columns"`
		katcTableDefinition
		Overlays []katcTableConfigOverlay `json:"overlays"`
	}

	katcTableConfigOverlay struct {
		Filters map[string]string `json:"filters"` // determines if this overlay is applicable to this launcher installation
		katcTableDefinition
	}

	katcTableDefinition struct {
		SourceType        *katcSourceType     `json:"source_type,omitempty"`
		SourcePaths       *[]string           `json:"source_paths,omitempty"` // Describes how to connect to source (e.g. path to db) -- % and _ wildcards supported
		SourceQuery       *string             `json:"source_query,omitempty"` // Query to run against each source path
		RowTransformSteps *[]rowTransformStep `json:"row_transform_steps,omitempty"`
	}
)

// ConstructKATCTables takes stored configuration of KATC tables, parses the configuration,
// and returns the constructed tables.
func ConstructKATCTables(config map[string]string, flags types.Flags, slogger *slog.Logger) []osquery.OsqueryPlugin {
	plugins := make([]osquery.OsqueryPlugin, 0)

	for tableName, tableConfigStr := range config {
		var cfg katcTableConfig
		if err := json.Unmarshal([]byte(tableConfigStr), &cfg); err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"unable to unmarshal config for KATC table, skipping",
				"table_name", tableName,
				"err", err,
			)
			continue
		}

		t, columns := newKatcTable(tableName, cfg, slogger)

		// Validate that the columns are valid for this table type -- only checked
		// for LevelDB tables currently
		if t.sourceType.name == leveldbSourceType {
			if err := validateLeveldbTableColumns(columns); err != nil {
				slogger.Log(context.TODO(), slog.LevelWarn,
					"invalid columns for leveldb table",
					"table_name", tableName,
					"err", err,
				)
				continue
			}
		}

		plugins = append(plugins, tablewrapper.New(flags, slogger, tableName, columns, t.generate))
	}

	return plugins
}
