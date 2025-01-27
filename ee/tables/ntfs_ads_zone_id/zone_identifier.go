//go:build windows
// +build windows

package ntfs_ads_zone_id

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_ntfs_ads_zone_id"),
	}

	return table.NewPlugin("kolide_ntfs_ads_zone_id", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	paths := tablehelpers.GetConstraints(queryContext, "path")
	if len(paths) < 1 {
		return nil, fmt.Errorf("kolide_ntfs_ads_zone_id requires at least one path to be specified")
	}

	for _, p := range paths {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			var output bytes.Buffer
			if err := tablehelpers.Run(ctx, t.slogger, 30, allowedcmd.Powershell, []string{"Get-Content", "-Path", path.Clean(p), "-Stream", "Zone.Identifier"}, &output, &output); err != nil {
				t.slogger.Log(ctx, slog.LevelInfo, "failure running powershell get-content command", "err", err, "path", p)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Ini(output.Bytes(), flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo, "failure flattening output", "err", err)
				continue
			}

			rowData := map[string]string{
				"path": p,
			}

			results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
		}
	}

	return results, nil
}
