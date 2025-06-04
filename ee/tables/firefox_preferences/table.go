package firefox_preferences

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	name    string
	slogger *slog.Logger
}

const tableName = "kolide_firefox_preferences"

// For the first iteration of this table, we decided to do our own parsing with regex,
// leaving the JSON strings as-is.
//
// input  -> user_pref("app.normandy.foo", "{\"abc\":123}");
// output -> [user_pref("app.normandy.foo", "{"abc":123}"); app.normandy.foo {"abc":123}]
//
// Note that we do not capture the surrounding quotes for either groups.
//
// In the future, we may want to use go-mozpref:
// https://github.com/hansmi/go-mozpref
var re = regexp.MustCompile(`^user_pref\("([^,]+)",\s*"?(.*?)"?\);$`)

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
	)

	t := &Table{
		name:    tableName,
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", tableName)
	defer span.End()

	var results []map[string]string

	filePaths := tablehelpers.GetConstraints(queryContext, "path")

	if len(filePaths) == 0 {
		t.slogger.Log(ctx, slog.LevelInfo,
			"no path provided",
		)
		return results, nil
	}

	for _, filePath := range filePaths {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			rawKeyVals, err := parsePreferences(filePath)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failed to parse preferences from file",
					"path", filePath,
					"err", err,
				)
				continue
			}

			flatData, err := dataflatten.Flatten(rawKeyVals, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelDebug,
					"failed to flatten data for path",
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

func parsePreferences(filePath string) (map[string]interface{}, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file at %s: %w", filePath, err)
	}
	defer file.Close()

	rawKeyVals := make(map[string]interface{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Given the line format: user_pref("app.normandy.first_run", false);
		// the return value should be a three element array, where the second
		// and third elements are the key and value, respectively.
		match := re.FindStringSubmatch(line)

		// If the match doesn't have a length of 3, the line is malformed in some way.
		// Skip it.
		if len(match) != 3 {
			continue
		}

		// The regex already stripped out the surrounding quotes, so now we're
		// left with escaped quotes that no longer make sense.
		// i.e. {\"249024122\":[1660860020218]}
		// Replace those with unescaped quotes.
		rawKeyVals[match[1]] = strings.ReplaceAll(match[2], "\\\"", "\"")
	}

	return rawKeyVals, nil
}
