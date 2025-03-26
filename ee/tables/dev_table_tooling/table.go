package dev_table_tooling

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

// allowedCommand encapsulates the possible binary path(s) of a command allowed to execute
// along with a strict list of arguments.
type allowedCommand struct {
	bin  allowedcmd.AllowedCommand
	args []string
}

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("args"),
		table.TextColumn("output"),
		table.TextColumn("error"),
	}

	tableName := "kolide_dev_table_tooling"

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_dev_table_tooling")
	defer span.End()

	var results []map[string]string

	for _, name := range tablehelpers.GetConstraints(queryContext, "name", tablehelpers.WithDefaults("")) {
		if name == "" {
			t.slogger.Log(ctx, slog.LevelInfo,
				"received blank command name, skipping",
			)
			continue
		}

		cmd, ok := allowedCommands[name]

		if !ok {
			t.slogger.Log(ctx, slog.LevelInfo,
				"command not allowed",
				"name", name,
			)
			continue
		}

		result := map[string]string{
			"name":   name,
			"args":   strings.Join(cmd.args, " "),
			"output": "",
			"error":  "",
		}

		if output, err := tablehelpers.RunSimple(ctx, t.slogger, 30, cmd.bin, cmd.args); err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"execution failed",
				"name", name,
				"err", err,
			)
			result["error"] = err.Error()
		} else {
			result["output"] = base64.StdEncoding.EncodeToString(output)
		}

		results = append(results, result)
	}

	return results, nil
}
