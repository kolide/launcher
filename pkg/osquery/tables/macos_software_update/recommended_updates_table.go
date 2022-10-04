//go:build darwin
// +build darwin

package macos_software_update

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <SUSharedPrefs.h>
#include "sus.h"
*/
import "C"
import (
	"context"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const softwareUpdateToolPath = "/usr/sbin/softwareupdate"
const softwareUpdateListArg = "--list"
const softwareUpdateNoScanArg = "--no-scan"

var updatesData []map[string]interface{}
var doNotScan bool

type Table struct {
	logger log.Logger
}

func RecommendedUpdates(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("noscan"),
	)

	tableName := "kolide_macos_recommended_updates"

	t := &Table{
		logger: log.With(logger, "table", tableName),
	}

	return table.NewPlugin(tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	var err error

	for _, noscan := range tablehelpers.GetConstraints(queryContext, "noscan", tablehelpers.WithDefaults("false")) {
		doNotScan, err = strconv.ParseBool(noscan)
		if err != nil {
			level.Info(t.logger).Log("msg", "Cannot convert noscan constraint into a boolean value. Try passing \"true\"", "err", err)
			continue
		}

		softwareUpdateArgs := []string{softwareUpdateListArg}

		if doNotScan {
			softwareUpdateArgs = append(softwareUpdateArgs, softwareUpdateNoScanArg)
		}

		_, err = tablehelpers.Exec(ctx, t.logger, 30, []string{softwareUpdateToolPath}, softwareUpdateArgs)
		if err != nil {
			level.Info(t.logger).Log("msg", "softwareupdate failed", "err", err)
			return nil, nil
		}

		data := getUpdates()

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattened, err := dataflatten.Flatten(data, dataflatten.WithLogger(t.logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
			if err != nil {
				level.Info(t.logger).Log("msg", "Error flattening data", "err", err)
				return nil, nil
			}
			results = append(results, dataflattentable.ToMap(flattened, dataQuery, nil)...)
		}
	}

	return results, nil
}

//export updatesFound
func updatesFound(numUpdates C.uint) {
	updatesData = make([]map[string]interface{}, numUpdates)
	for i := 0; i < int(numUpdates); i++ {
		updatesData[i] = map[string]interface{}{"noscan": strconv.FormatBool(doNotScan)}
	}
}

//export updateKeyValueFound
func updateKeyValueFound(index C.uint, key, value *C.char) {
	if updatesData[index] == nil {
		updatesData[index] = make(map[string]interface{})
	}
	updatesData[index][C.GoString(key)] = C.GoString(value)
}

func getUpdates() map[string]interface{} {
	results := make(map[string]interface{})

	C.getRecommendedUpdates()
	results["RecommendedUpdates"] = updatesData

	return results
}
