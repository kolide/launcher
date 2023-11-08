//go:build windows
// +build windows

package dsim_default_associations

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/allowedpaths"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	logger log.Logger
}

func TablePlugin(logger log.Logger) *table.Plugin {

	columns := dataflattentable.Columns()

	t := &Table{
		logger: logger,
	}

	return table.NewPlugin("kolide_dsim_default_associations", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	dismResults, err := t.execDism(ctx)
	if err != nil {
		level.Info(t.logger).Log("msg", "dism failed", "err", err)
		return results, err
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithLogger(t.logger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		rows, err := dataflatten.Xml(dismResults, flattenOpts...)
		if err != nil {
			level.Info(t.logger).Log("msg", "flatten failed", "err", err)
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

	dstFile := "associations.xml"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := []string{"/online", "/Export-DefaultAppAssociations:" + dstFile}

	cmd, err := allowedpaths.Dism(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("creating command: %w", err)
	}
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling dism", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("calling dism. Got: %s: %w", stderr.String(), err)
	}

	data, err := os.ReadFile(filepath.Join(dir, dstFile))
	if err != nil {
		return nil, fmt.Errorf("error reading dism output file: %s: %w", err, err)
	}

	return data, nil
}
