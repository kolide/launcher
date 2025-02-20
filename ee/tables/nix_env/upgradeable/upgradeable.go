//go:build linux
// +build linux

package nix_env_upgradeable

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "0123456789"

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("uid"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_nix_upgradeable"),
	}

	return tablewrapper.New(flags, slogger, "kolide_nix_upgradeable", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_nix_upgradeable")
	defer span.End()

	var results []map[string]string

	uids := tablehelpers.GetConstraints(queryContext, "uid", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(uids) < 1 {
		return results, errors.New("kolide_nix_upgradeable requires at least one user id to be specified")
	}

	for _, uid := range uids {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			// Nix takes a while to load, so leaving a minute timeout here to give enough time. More might be needed.

			var output bytes.Buffer
			if err := tablehelpers.Run(ctx, t.slogger, 60, allowedcmd.NixEnv, []string{"--query", "--upgradeable", "-c", "--xml"}, &output, &output, tablehelpers.WithUid(uid)); err != nil {
				t.slogger.Log(ctx, slog.LevelInfo, "failure querying user installed packages", "err", err, "target_uid", uid)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Xml(output.Bytes(), flattenOpts...)
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
