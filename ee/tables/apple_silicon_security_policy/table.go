//go:build darwin
// +build darwin

package apple_silicon_security_policy

import (
	"bytes"
	"context"
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

const bootPolicyUtilArgs = "--display-all-policies"

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	tableName := "kolide_apple_silicon_security_policy"

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_apple_silicon_security_policy")
	defer span.End()

	var results []map[string]string

	output, err := tablehelpers.RunSimple(ctx, t.slogger, 30, allowedcmd.Bputil, []string{bootPolicyUtilArgs})
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"bputil failed",
			"err", err,
		)
		return nil, nil
	}

	if len(output) == 0 {
		t.slogger.Log(ctx, slog.LevelInfo,
			"no bputil data to parse",
		)
		return nil, nil
	}

	data := parseBootPoliciesOutput(bytes.NewReader(output))

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattened, err := dataflatten.Flatten(data, dataflatten.WithSlogger(t.slogger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error flattening data",
				"err", err,
			)
			return nil, nil
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}
