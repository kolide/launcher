package firefox_prefs

import (
	"bufio"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	name	 string
	logger log.Logger
}

const tableName = "kolide_firefox_prefs"

func TablePlugin(_ *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		name:		tableName,
		logger: logger,
	}

	return table.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	usr, _ := user.Current()
	return output(filepath.Join(usr.HomeDir, "github/launcher/prefs.js"), queryContext, t.logger)
}

func output(path string, queryContext table.QueryContext, logger log.Logger) ([]map[string]string, error)  {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	rowData := make(map[string]string)
	var results []map[string]string
	m := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()
		re := regexp.MustCompile(`user_pref\((.*)\)`)
		match := re.FindStringSubmatch(line)

		if len(match) > 1 {
			parts := strings.Split(match[1], ", ")
			m[parts[0]] = parts[1]
		}
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

		flattened, err := dataflatten.Flatten(m, dataflatten.WithLogger(logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			return nil, err
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}
