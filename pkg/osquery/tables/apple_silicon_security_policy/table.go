//go:build darwin
// +build darwin

package apple_silicon_security_policy

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const bootPolicyUtilPath = "/usr/bin/bputil"
const bootPolicyUtilArgs = "--display-all-policies"

type Table struct {
	logger log.Logger
}

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

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
		return nil, nil
	}

	status, err := generatePoliciesTable(output, queryContext, t.logger)
	if err != nil {
		level.Info(t.logger).Log("msg", "Error parsing status", "err", err)
		return nil, nil
	}

	results = status

	return results, nil
}

func generatePoliciesTable(rawdata []byte, queryContext table.QueryContext, logger log.Logger) ([]map[string]string, error) {
	data := []map[string]string{}

	if len(rawdata) == 0 {
		return nil, errors.New("No data")
	}

	var output map[string]interface{}
	rowData := map[string]string{}

	output = parseBootPoliciesOutput(bytes.NewReader(rawdata))

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattened, err := dataflatten.Flatten(output, dataflatten.WithLogger(logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			return nil, err
		}
		data = append(data, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return data, nil
}
