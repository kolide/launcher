package falconctlstats

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

type Table struct {
	name   string
	logger log.Logger
}

// TODO: Change table name to kolide_falconctl_stats
const tableName = "kolide_crowdstrike"

var re = regexp.MustCompile(`\s?=\s?|\s`)

func TablePlugin(logger log.Logger) *osquerygotable.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		name:   tableName,
		logger: logger,
	}

	return osquerygotable.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext osquerygotable.QueryContext) ([]map[string]string, error) {
	// TODO: Replace this with the output of the real command
	// output := "aid is not set, aph is not set, app is not set, rfm-state is not set, rfm-reason is not set, feature is not set, metadata-query=enable (unset default), version = 6.38.13501.0"

	options := []string{
		"-g",
		"--rfm-state",
		"--aid",
		"--version",
		"--metadata-query",
		"--rfm-reason",
		"--tags",
		"--feature",
		"--app",
		"--aph",
		"--provisioning-token",
		"--systags",
		"--cid",
	}
	output, err := tablehelpers.Exec(context.Background(), t.logger, 30, []string{"/opt/CrowdStrike/falconctl"}, options)
	if err != nil {
		panic(err)
	}

	return parse(queryContext, t.logger, string(output))
}

func parse(queryContext osquerygotable.QueryContext, logger log.Logger, output string) ([]map[string]string, error) {
	// TODO: Add case for when CID not set
	var results []map[string]string

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		dataArr := strings.Split(output, ", ")
		rawKeyVals := make(map[string]interface{})

		for _, data := range dataArr {
			a := re.Split(data, 2)
			rawKeyVals[a[0]] = a[1]
		}

		flatData, err := dataflatten.Flatten(rawKeyVals, flattenOpts...)
		if err != nil {
			level.Debug(logger).Log(
				"msg", "failed to flatten data",
				"table", tableName,
				"err", err,
			)
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, make(map[string]string))...)
	}

	return results, nil
}
