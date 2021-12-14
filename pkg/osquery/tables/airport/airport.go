//go:build darwin
// +build darwin

package airport

import (
	"context"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

var (
	allowedOptions = []string{"getinfo", "scan"}
	airportPaths   = []string{"/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"}
)

type Table struct {
	name   string
	logger log.Logger
}

func TablePlugin(_ *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("option"),
	)

	t := &Table{
		name:   "kolide_airport_util",
		logger: logger,
	}

	return table.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	options := tablehelpers.GetConstraints(queryContext, "option", tablehelpers.WithAllowedValues(allowedOptions))

	if len(options) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a constraint for option", t.name)
	}

	for _, option := range options {
		airportOutput, err := tablehelpers.Exec(ctx, t.logger, 30, airportPaths, []string{"--" + option, "--xml"})
		if err != nil {
			level.Debug(t.logger).Log("msg", "Error execing airport", "option", option, "err", err)
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flatData, err := dataflatten.Xml(
				airportOutput,
				dataflatten.WithLogger(t.logger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			)

			if err != nil {
				level.Info(t.logger).Log("msg", "flatten failed", "err", err)
				continue
			}

			rowData := map[string]string{"option": option}
			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)

		}
	}
	return results, nil
}
