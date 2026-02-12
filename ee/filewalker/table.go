package filewalker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type filewalkTable struct {
	resultsKey      []byte
	lastWalkTimeKey []byte
	resultsStore    types.Getter
	slogger         *slog.Logger
}

func NewFilewalkTable(tableName string, flags types.Flags, resultsStore types.Getter, slogger *slog.Logger) osquery.OsqueryPlugin {
	ft := &filewalkTable{
		resultsKey:      []byte(tableName),
		lastWalkTimeKey: LastWalkTimeKey(tableName),
		resultsStore:    resultsStore,
		slogger:         slogger.With("table", tableName),
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.IntegerColumn("last_walk_timestamp"),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, ft.generate)
}

func (ft *filewalkTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	lastWalkTimeRaw, err := ft.resultsStore.Get(ft.lastWalkTimeKey)
	if err != nil {
		return nil, fmt.Errorf("retrieving last walk time from store: %w", err)
	}
	lastWalkTime := "0"
	if lastWalkTimeRaw != nil {
		lastWalkTime = strconv.FormatInt(int64(binary.NativeEndian.Uint64(lastWalkTimeRaw)), 10)
	}

	rawResults, err := ft.resultsStore.Get(ft.resultsKey)
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
			"path":                path,
			"last_walk_timestamp": lastWalkTime,
		}
	}

	return rows, nil
}
