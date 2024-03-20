//go:build !windows
// +build !windows

package brew_upgradeable

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "0123456789"

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("uid"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_brew_upgradeable"),
	}

	return table.NewPlugin("kolide_brew_upgradeable", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	uids := tablehelpers.GetConstraints(queryContext, "uid", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(uids) < 1 {
		return results, fmt.Errorf("kolide_brew_upgradeable requires at least one user id to be specified")
	}

	for _, uid := range uids {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			output, err := tablehelpers.Exec(ctx, t.slogger, 30, allowedcmd.Brew, []string{"outdated", "--json"}, true, tablehelpers.WithUid(uid))
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo, "failure querying user brew installed packages", "err", err, "target_uid", uid)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Json(output, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo, "failure flattening output", "err", err)
				continue
			}

			rowData := map[string]string{
				"uid": uid,
			}

			results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
		}
	}

	return results, nil
}
