package dev_table_tooling

import (
	"context"
	"encoding/base64"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

// Encapsulates the binary path(s) of a command allowed to execute
// along with a strict list of arguments.
type AllowedCommand struct {
	binPaths []string
	args     []string
}

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("output"),
	}

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_dev_table_tooling",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, name := range tablehelpers.GetConstraints(queryContext, "name", tablehelpers.WithDefaults("")) {
		if name == "" {
			level.Info(t.logger).Log("msg", "Command name must not be blank")
			continue
		}

		cmd, ok := GetAllowedCommands()[name]

		if !ok {
			level.Info(t.logger).Log("msg", "Command not allowed", "name", name)
			continue
		} else {
			output, err := tablehelpers.Exec(ctx, t.logger, 30, cmd.binPaths, cmd.args)
			if err != nil {
				level.Info(t.logger).Log("msg", "dev_table_tooling failed", "name", name, "err", err)
				continue
			} else {
				results = append(results, map[string]string{
					"name":   name,
					"output": base64.StdEncoding.EncodeToString(output),
				})
			}
		}
	}

	return results, nil
}
