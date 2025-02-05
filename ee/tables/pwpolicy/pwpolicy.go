//go:build darwin
// +build darwin

// Package pwpolicy provides a table wrapper around the `pwpolicy` macOS
// command.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/ee/dataflatten)
package pwpolicy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const pwpolicyCmd = "getaccountpolicies"

type Table struct {
	slogger   *slog.Logger
	tableName string
	execCC    allowedcmd.AllowedCommand
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("username"),
	)

	t := &Table{
		slogger:   slogger.With("table", "kolide_pwpolicy"),
		tableName: "kolide_pwpolicy",
		execCC:    allowedcmd.Pwpolicy,
	}

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	var results []map[string]string

	for _, pwpolicyUsername := range tablehelpers.GetConstraints(queryContext, "username", tablehelpers.WithDefaults("")) {
		pwpolicyArgs := []string{pwpolicyCmd}

		if pwpolicyUsername != "" {
			pwpolicyArgs = append(pwpolicyArgs, "-u", pwpolicyUsername)
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			pwPolicyOutput, err := t.execPwpolicy(ctx, pwpolicyArgs)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"pwpolicy failed",
					"err", err,
				)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flatData, err := dataflatten.Plist(pwPolicyOutput, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"flatten failed",
					"err", err,
				)
				continue
			}

			rowData := map[string]string{
				"username": pwpolicyUsername,
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}

	return results, nil
}

func (t *Table) execPwpolicy(ctx context.Context, args []string) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	t.slogger.Log(ctx, slog.LevelDebug,
		"calling pwpolicy",
		"args", args,
	)

	if err := tablehelpers.Run(ctx, t.slogger, 30, t.execCC, args, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("calling pwpolicy. Got: %s: %w", stderr.String(), err)
	}

	// Remove first line of output because it always contains non-plist content
	outputBytes := bytes.SplitAfterN(stdout.Bytes(), []byte("\n"), 2)[1]

	return outputBytes, nil
}
