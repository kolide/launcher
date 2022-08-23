package firefox_preferences

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	name   string
	logger log.Logger
}

const tableName = "kolide_firefox_preferences"

func TablePlugin(_ *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
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
	return generateData(queryContext, t.logger)
}

func generateData(queryContext table.QueryContext, logger log.Logger) ([]map[string]string, error) {
	paths := tablehelpers.GetConstraints(queryContext, "path")

	if len(paths) != 1 {
		return nil, errors.Errorf("The %s table requires that you specify a constraint for path", tableName)
	}

	file, err := os.Open(paths[0])
	if err != nil {
		// TODO: Investigate what error message looks like. Add filepath possibly
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	rowData := map[string]string{"path": paths[0]}
	rawKeyVals := make(map[string]interface{})

	re := regexp.MustCompile(`user_pref\((.*)\)`)
	for scanner.Scan() {
		line := scanner.Text()
		match := re.FindStringSubmatch(line)

		if len(match) <= 1 {
			continue
		}

		parts := strings.Split(match[1], ", ")
		rawKeyVals[parts[0]] = parts[1]
	}

	var results []map[string]string
	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

		flattened, err := dataflatten.Flatten(rawKeyVals, dataflatten.WithLogger(logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			return nil, err
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}
