//go:build darwin
// +build darwin

package apple_silicon_security_policy

import (
	"bytes"
	"context"
	"log/slog"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const bootPolicyUtilArgs = "--display-all-policies"

type Table struct {
	logger  log.Logger // preserved only for temporary use in dataflattentable and tablehelpers.Exec
	slogger *slog.Logger
}

func TablePlugin(slogger *slog.Logger, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	tableName := "kolide_apple_silicon_security_policy"

	t := &Table{
		slogger: slogger.With("table", tableName),
		logger:  log.With(logger, "table", tableName),
	}

	return table.NewPlugin(tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	output, err := tablehelpers.Exec(ctx, t.logger, 30, allowedcmd.Bputil, []string{bootPolicyUtilArgs}, false)
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
		flattened, err := dataflatten.Flatten(data, dataflatten.WithLogger(t.logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
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
