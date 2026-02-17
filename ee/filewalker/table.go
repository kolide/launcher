package filewalker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type filewalkTable struct {
	resultsStore types.Getter
	slogger      *slog.Logger
}

func NewFilewalkTable(flags types.Flags, resultsStore types.Getter, slogger *slog.Logger) osquery.OsqueryPlugin {
	ft := &filewalkTable{
		resultsStore: resultsStore,
		slogger:      slogger.With("table", "kolide_filewalk"),
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("walk_name"),
		table.TextColumn("path"),
		table.IntegerColumn("last_walk_timestamp"),
	}

	return tablewrapper.New(flags, slogger, "kolide_filewalk", columns, ft.generate)
}

func (ft *filewalkTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	walkNames := tablehelpers.GetConstraints(queryContext, "walk_name")
	if len(walkNames) == 0 {
		return nil, errors.New("the kolide_filewalk table requires that you specify an equals constraint for walk_name")
	}

	results := make([]map[string]string, 0)

	for _, walkName := range walkNames {
		lastWalkTimeRaw, err := ft.resultsStore.Get(LastWalkTimeKey(walkName))
		if err != nil {
			ft.slogger.Log(ctx, slog.LevelWarn,
				"could not retrieve last walk time from store",
				"walk_name", walkName,
				"err", err,
			)
			continue
		}
		lastWalkTime := "0"
		if lastWalkTimeRaw != nil {
			lastWalkTime = strconv.FormatInt(int64(binary.NativeEndian.Uint64(lastWalkTimeRaw)), 10)
		}

		rawResults, err := ft.resultsStore.Get([]byte(walkName))
		if err != nil {
			ft.slogger.Log(ctx, slog.LevelWarn,
				"could not retrieve rows from store",
				"walk_name", walkName,
				"err", err,
			)
			continue
		}

		// No results, or none stored yet
		if rawResults == nil {
			continue
		}

		paths := make([]string, 0)
		if err := json.Unmarshal(rawResults, &paths); err != nil {
			ft.slogger.Log(ctx, slog.LevelWarn,
				"could not unmarshal rows from store",
				"walk_name", walkName,
				"err", err,
			)
			continue
		}
		for _, path := range paths {
			results = append(results, map[string]string{
				"walk_name":           walkName,
				"path":                path,
				"last_walk_timestamp": lastWalkTime,
			})
		}
	}

	return results, nil
}
