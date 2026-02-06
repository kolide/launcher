package filewalker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type filewalkTable struct {
	storeKey     []byte
	resultsStore types.Getter
	slogger      *slog.Logger
}

func NewFilewalkTable(tableName string, flags types.Flags, resultsStore types.Getter, slogger *slog.Logger) osquery.OsqueryPlugin {
	ft := &filewalkTable{
		storeKey:     []byte(tableName),
		resultsStore: resultsStore,
		slogger:      slogger.With("table", tableName),
	}
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
	)

	return tablewrapper.New(flags, slogger, tableName, columns, ft.generate)
}

func (ft *filewalkTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	rawResults, err := ft.resultsStore.Get(ft.storeKey)
	if err != nil {
		return nil, fmt.Errorf("retrieving rows from store: %w", err)
	}

	// No results, or none stored yet
	if rawResults == nil {
		return nil, nil
	}

	paths := make([]string, 0)
	if err := json.Unmarshal(rawResults, &paths); err != nil {
		return nil, fmt.Errorf("unmarshalling results from store: %w", err)
	}

	rows := make([]map[string]string, len(paths))
	for i, path := range paths {
		rows[i] = map[string]string{
			"path": path,
		}
	}

	return rows, nil
}
