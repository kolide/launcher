//go:build windows
// +build windows

package dism_default_associations

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
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

	columns := dataflattentable.Columns()

	t := &Table{
		slogger: slogger.With("table", "kolide_dsim_default_associations"),
	}

	return table.NewPlugin("kolide_dsim_default_associations", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
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
	// dism.exe outputs xml, but with weird intermingled status. So
	// instead, we dump it to a temp file.
	dir, err := agent.MkdirTemp("kolide_dism")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_dism tmp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	const dstFile = "associations.xml"
	args := []string{"/online", "/Export-DefaultAppAssociations:" + dstFile}

	out, err := tablehelpers.Exec(ctx, t.slogger, 30, allowedcmd.Dism, args, true, tablehelpers.WithDir(dir))
	if err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"execing dism",
			"args", args,
			"out", string(out),
			"err", err,
		)

		return nil, fmt.Errorf("execing dism: out: %s, %w", string(out), err)
	}

	data, err := os.ReadFile(filepath.Join(dir, dstFile))
	if err != nil {
		return nil, fmt.Errorf("reading dism output file: %w", err)
	}

	return data, nil
}
