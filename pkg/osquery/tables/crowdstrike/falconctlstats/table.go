// TODO: Add linux build comments
package falconctlstats

import (
	"context"
	"encoding/json"
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

var re = regexp.MustCompile(`\s?=\s?|\sis\s`)

func TablePlugin(logger log.Logger) *osquerygotable.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		name:   tableName,
		logger: logger,
	}

	return osquerygotable.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext osquerygotable.QueryContext) ([]map[string]string, error) {
	output := `cid="c1196fd3ca9944f39681d1c9c49124a7", aid="c2bbdbb81bc84d3c9bcb1542f16fb020", apd is not set, aph is not set, app is not set, rfm-state=true, rfm-reason=Unspecified, code=0xC0000225, trace is not set, feature= (hex bitmask: 0), billing is not set, tags=kolide-test-1,kolide-test-2, provisioning-token is not set.`

	// options := []string{
	// 	"-g",
	// 	"--cid",
	// 	"--aid",
	// 	"--apd",
	// 	"--aph",
	// 	"--app",
	// 	"--rfm-state",
	// 	"--rfm-reason",
	// 	"--trace",
	// 	"--feature",
	// 	"--metadata-query",
	// 	"--version",
	// 	"--message-log",
	// 	"--billing",
	// 	"--tags",
	// 	"--provisioning-token",
	// 	"--systags",
	// }
	// output, err := tablehelpers.Exec(context.Background(), t.logger, 30, []string{"/opt/CrowdStrike/falconctl"}, options)
	// if err != nil {
	// 	panic(err)
	// }

	return parse(queryContext, t.logger, string(output))
}

func parse(queryContext osquerygotable.QueryContext, logger log.Logger, output string) ([]map[string]string, error) {
	// TODO: Only parse first line
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
			if a[0] == "tags" {
				rawKeyVals[a[0]] = strings.Split(a[1], ",")
			} else {
				rawKeyVals[a[0]] = a[1]
			}
		}

		input, _ := json.Marshal(rawKeyVals)
		flatData, err := dataflatten.Json(input, flattenOpts...)
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
