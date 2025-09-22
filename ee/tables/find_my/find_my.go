//go:build darwin
// +build darwin

package find_my

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c -fmodules
#cgo darwin LDFLAGS: -framework Cocoa
#include "fmd.h"
*/
import (
	"C"
)
import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type findMyDeviceTable struct {
	slogger   *slog.Logger
	tableName string
}

func FindMyDevice(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("find_my_mac_enabled"),
	}

	tableName := "kolide_find_my"

	t := &findMyDeviceTable{
		slogger:   slogger.With("table", tableName),
		tableName: tableName,
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (f *findMyDeviceTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	_, span := observability.StartSpan(ctx, "table_name", f.tableName)
	defer span.End()

	var findMyMacEnabled = C.int(0)
	C.getFMDSettings(&findMyMacEnabled)

	results := []map[string]string{
		{
			"find_my_mac_enabled": fmt.Sprintf("%d", findMyMacEnabled),
		},
	}

	return results, nil
}
