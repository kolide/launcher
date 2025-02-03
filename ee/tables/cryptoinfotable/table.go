package cryptoinfotable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/cryptoinfo"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("passphrase"),
		table.TextColumn("path"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_cryptinfo"),
	}

	return tablewrapper.New(flags, slogger, "kolide_cryptinfo", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_cryptinfo")
	defer span.End()

	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	if len(requestedPaths) == 0 {
		return results, errors.New("The kolide_cryptoinfo table requires that you specify an equals constraint for path")
	}

	for _, requestedPath := range requestedPaths {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"bad file glob",
				"err", err,
			)
			continue
		}

		for _, filePath := range filePaths {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				for _, passphrase := range tablehelpers.GetConstraints(queryContext, "passphrase", tablehelpers.WithDefaults("")) {

					flattenOpts := []dataflatten.FlattenOpts{
						dataflatten.WithSlogger(t.slogger),
						dataflatten.WithNestedPlist(),
						dataflatten.WithQuery(strings.Split(dataQuery, "/")),
					}

					flatData, err := flattenCryptoInfo(filePath, passphrase, flattenOpts...)
					if err != nil {
						t.slogger.Log(ctx, slog.LevelInfo,
							"failed to get data for path",
							"path", filePath,
							"err", err,
						)
						continue
					}

					rowData := map[string]string{
						"path":       filePath,
						"passphrase": passphrase,
					}
					results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)

				}
			}
		}
	}
	return results, nil
}

// flattenCryptoInfo is a small wrapper over ee/cryptoinfo that passes it off to dataflatten for table generation
func flattenCryptoInfo(filename, passphrase string, opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	filebytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}

	result, err := cryptoinfo.Identify(filebytes, passphrase)
	if err != nil {
		return nil, fmt.Errorf("parsing with cryptoinfo: %w", err)
	}

	// convert to json, so it's parsable
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	return dataflatten.Json(jsonBytes, opts...)
}
