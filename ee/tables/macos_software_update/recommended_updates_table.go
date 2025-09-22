//go:build darwin
// +build darwin

package macos_software_update

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#include "sus.h"
*/
import (
	"C"
)
import (
	"context"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

var updatesData []map[string]interface{}

type Table struct {
	slogger *slog.Logger
}

func RecommendedUpdates(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	tableName := "kolide_macos_recommended_updates"

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_macos_recommended_updates")
	defer span.End()

	var results []map[string]string

	data := getUpdates(ctx)

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattened, err := dataflatten.Flatten(data, dataflatten.WithSlogger(t.slogger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error flattening data",
				"err", err,
			)
			return nil, nil
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}

//export updatesFound
func updatesFound(numUpdates C.uint) {
	// getRecommendedUpdates will use this callback to indicate how many updates have been found
	updatesData = make([]map[string]interface{}, numUpdates)
}

//export updateKeyValueFound
func updateKeyValueFound(index C.uint, key, value *C.char) {
	// getRecommendedUpdates will use this callback for each key-value found
	if updatesData[index] == nil {
		updatesData[index] = make(map[string]interface{})
	}
	updatesData[index][C.GoString(key)] = C.GoString(value)
}

func getUpdates(ctx context.Context) map[string]interface{} {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	results := make(map[string]interface{})

	// Since updatesData is package level, reset it before each invocation to purge stale results
	updatesData = nil

	C.getRecommendedUpdates()
	results["RecommendedUpdates"] = updatesData

	return results
}
