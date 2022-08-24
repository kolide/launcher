package firefox_preferences

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	name   string
	logger log.Logger
}

const tableName = "kolide_firefox_preferences"

var re = regexp.MustCompile(`^user_pref\("([^,]+)",\s*(.*)"?\);$`)

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
	return generateData(queryContext, t.logger)
}

func generateData(queryContext table.QueryContext, logger log.Logger) ([]map[string]string, error) {
	var results []map[string]string

	filePaths := tablehelpers.GetConstraints(queryContext, "path")

	if len(filePaths) == 0 {
		return nil, errors.Errorf("The %s table requires that you specify a constraint WHERE path IN", tableName)
	}

	rawKeyVals := make(map[string]interface{})

	for _, filePath := range filePaths {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithLogger(logger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			file, err := os.Open(filePath)
			if err != nil {
				level.Info(logger).Log(
					"msg", "failed to open file",
					"path", filePath,
					"err", err,
				)
				continue
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				match := re.FindStringSubmatch(line)

				if len(match) != 3 {
					continue
				}

				rawKeyVals[match[1]] = match[2]
			}

			flatData, err := dataflatten.Flatten(rawKeyVals, flattenOpts...)
			if err != nil {
				level.Info(logger).Log(
					"msg", "failed to get data for path",
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
