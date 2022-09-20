package falconctl

import (
	"context"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	osquerygotable "github.com/osquery/osquery-go/plugin/table"
)

type table struct {
	name   string
	logger log.Logger
}

const tableName = "kolide_crowdstrike"

var re = regexp.MustCompile(`\s?=\s?|\s`)

func tablePlugin(logger log.Logger) *osquerygotable.Plugin {
	columns := dataflattentable.Columns()

	t := &table{
		name:   tableName,
		logger: logger,
	}

	return osquerygotable.NewPlugin(t.name, columns, t.generate)
}

func (t *table) generate(ctx context.Context, queryContext osquerygotable.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		// TODO: Replace this with the output of the real command
		output := "aid is not set, aph is not set, app is not set, rfm-state is not set, rfm-reason is not set, feature is not set, metadata-query=enable (unset default), version = 6.38.13501.0"
		dataArr := strings.Split(output, ", ")
		rawKeyVals := make(map[string]interface{})

		for _, data := range dataArr {
			a := re.Split(data, 2)
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
