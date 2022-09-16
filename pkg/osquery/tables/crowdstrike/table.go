package crowdstrike

import (
	"context"
	"regexp"
	"strings"

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

const tableName = "kolide_crowdstrike"

var re = regexp.MustCompile(`\s?=\s?|\s`)

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		name:   tableName,
		logger: logger,
	}

	return table.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		str := "aid is not set, aph is not set, app is not set, rfm-state is not set, rfm-reason is not set, feature is not set, metadata-query=enable (unset default), version = 6.38.13501.0"
		arr := strings.Split(str, ", ")
		rawKeyVals := make(map[string]interface{})

		for _, foo := range arr {
			a := re.Split(foo, 2)
			rawKeyVals[a[0]] = a[1]
		}

		flatData, err := dataflatten.Flatten(rawKeyVals, flattenOpts...)
		if err != nil {
			level.Debug(t.logger).Log(
				"msg", "failed to flatten data",
				"table", tableName,
				"err", err,
			)
		}

		rowData := make(map[string]string)
		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}

	return results, nil
}
