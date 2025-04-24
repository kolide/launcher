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
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

var productsData []map[string]interface{}
var cachedTime time.Time

func AvailableProducts(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	tableName := "kolide_macos_available_products"

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generateAvailableProducts)
}

func (t *Table) generateAvailableProducts(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_macos_available_products")
	defer span.End()

	var results []map[string]string

	data := getProducts(ctx)

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

//export productsFound
func productsFound(numProducts C.uint) {
	// getAvailableProducts will use this callback to indicate how many products have been found
	productsData = make([]map[string]interface{}, numProducts)
}

//export productKeyValueFound
func productKeyValueFound(index C.uint, key, value *C.char) {
	// getAvailableProducts will use this callback for each key-value found
	if productsData[index] == nil {
		productsData[index] = make(map[string]interface{})
	}
	if value != nil {
		productsData[index][C.GoString(key)] = C.GoString(value)
	}
}

//export productNestedKeyValueFound
func productNestedKeyValueFound(index C.uint, parent, key, value *C.char) {
	// getAvailableProducts will use this callback for each nested key-value found
	if productsData[index] == nil {
		productsData[index] = make(map[string]interface{})
	}

	parentStr := C.GoString(parent)
	if productsData[index][parentStr] == nil {
		productsData[index][parentStr] = make(map[string]interface{})
	}

	if value != nil {
		parentObj, _ := productsData[index][parentStr].(map[string]interface{})
		parentObj[C.GoString(key)] = C.GoString(value)
	}
}

func getProducts(ctx context.Context) map[string]interface{} {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	results := make(map[string]interface{})

	// Calling getAvailableProducts is an expensive operation and could cause performance
	// problems if called too frequently. Here we cache the data and restrict the
	// frequency of invocations to at most once per minute.
	if productsData != nil && time.Since(cachedTime) < 1*time.Minute {
		results["AvailableProducts"] = productsData
		return results
	}

	// Since productsData is package level, reset it before each invocation to purge stale results
	productsData = nil

	C.getAvailableProducts()

	// Remember when we last retrieved the data
	cachedTime = time.Now()

	results["AvailableProducts"] = productsData
	return results
}
