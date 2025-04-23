//go:build windows
// +build windows

package dsim_default_associations

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns()

	t := &Table{
		slogger: slogger.With("table", "kolide_dsim_default_associations"),
	}

	return tablewrapper.New(flags, slogger, "kolide_dsim_default_associations", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_dsim_default_associations")
	defer span.End()

	var results []map[string]string

	dismResults, err := t.execDism(ctx)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"dsim failed",
			"err", err,
		)
		return results, err
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithSlogger(t.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		rows, err := dataflatten.Xml(dismResults, flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"flatten failed",
				"err", err,
			)
			continue
		}

		results = append(results, dataflattentable.ToMap(rows, dataQuery, nil)...)
	}

	return results, nil
}

func (t *Table) execDism(ctx context.Context) ([]byte, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// dism.exe outputs xml, but with weird intermingled status. So
	// instead, we dump it to a temp file.
	dir, err := agent.MkdirTemp("kolide_dism")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_dism tmp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	dstFile := "associations.xml"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{"/online", "/Export-DefaultAppAssociations:" + dstFile}

	t.slogger.Log(ctx, slog.LevelDebug,
		"calling dsim",
		"args", args,
	)

	if err := tablehelpers.Run(ctx, t.slogger, 30, allowedcmd.Dism, args, &stdout, &stderr, tablehelpers.WithDir(dir)); err != nil {
		return nil, fmt.Errorf("calling dism. Got: %s: %w", stderr.String(), err)
	}

	data, err := os.ReadFile(filepath.Join(dir, dstFile))
	if err != nil {
		return nil, fmt.Errorf("error reading dism output file: %s: %w", err, err)
	}

	return data, nil
}
