//go:build darwin
// +build darwin

package apple_silicon_security_policy

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const bootPolicyUtilPath = "/usr/bin/bputil"
const bootPolicyUtilArgs = "--display-all-policies"

type Table struct {
	logger log.Logger
}

func TablePlugin(logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("volume_group"),
		table.TextColumn("property"),
		table.TextColumn("mode"),
		table.TextColumn("code"),
		table.TextColumn("value"),
	}

	tableName := "apple_silicon_security_policy"

	t := &Table{
		logger: log.With(logger, "table", tableName),
	}

	return table.NewPlugin(tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	output, err := tablehelpers.Exec(ctx, t.logger, 30, []string{bootPolicyUtilPath}, []string{bootPolicyUtilArgs})
	if err != nil {
		level.Info(t.logger).Log("msg", "bputil failed", "err", err)
		return results, err
	}

	status, err := parseStatus(output)
	if err != nil {
		level.Info(t.logger).Log("msg", "Error parsing status", "err", err)
		return results, err
	}

	results = status

	return results, nil
}
