package syft

import (
	"context"
	"fmt"
	"strings"

	"github.com/anchore/syft/syft"
	"github.com/anchore/syft/syft/formats/syftjson"
	"github.com/anchore/syft/syft/pkg/cataloger"
	"github.com/anchore/syft/syft/sbom"
	"github.com/anchore/syft/syft/source"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	name   string
	logger log.Logger
}

const tableName = "kolide_syft"

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
	)

	t := &Table{
		name:   tableName,
		logger: logger,
	}

	return table.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	filePaths := tablehelpers.GetConstraints(queryContext, "path")

	if len(filePaths) == 0 {
		level.Info(t.logger).Log(
			"msg", fmt.Sprintf("no path provided to %s", tableName),
			"table", tableName,
		)
		return results, nil
	}

	for _, filePath := range filePaths {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			src, srcCleanup := source.NewFromFile(filePath)
			if srcCleanup != nil {
				defer srcCleanup()
			}

			packageCatalog, relationships, linuxDistro, err := syft.CatalogPackages(&src, cataloger.DefaultConfig())
			if err != nil {
				level.Info(t.logger).Log(
					"msg", "cataloging packages with syft",
					"table", tableName,
					"path", filePath,
					"err", err,
				)
				continue
			}

			sbom := sbom.SBOM{
				Source: src.Metadata,
				Descriptor: sbom.Descriptor{
					Name: "syft",
				},
			}

			sbom.Artifacts.PackageCatalog = packageCatalog
			sbom.Relationships = relationships
			sbom.Artifacts.LinuxDistribution = linuxDistro

			jsonBytes, err := syft.Encode(sbom, syftjson.Format())
			if err != nil {
				level.Error(t.logger).Log(
					"msg", "encoding syft sbom to json",
					"table", tableName,
					"path", filePath,
					"err", err,
				)
				continue
			}

			flatData, err := dataflatten.Json(jsonBytes, flattenOpts...)
			if err != nil {
				level.Debug(t.logger).Log(
					"msg", "flattening syft sbom json",
					"table", tableName,
					"path", filePath,
					"err", err,
				)
				continue
			}

			rowData := map[string]string{"path": filePath}
			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}

	return results, nil
}
