package dataflattentable

import (
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func NewExecAndParseTable(
	logger log.Logger,
	tableName string,
	parser parserInt,
	execArgs []string,
	opts ...ExecTableOpt,

) *table.Plugin {
	t := &Table{
		logger:     level.NewFilter(log.With(logger, "table", tableName), level.AllowInfo()),
		tableName:  tableName,
		dataParser: parser,
		execArgs:   execArgs,
	}

	for _, opt := range opts {
		opt(t)
	}

	return table.NewPlugin(t.tableName, Columns(), t.generateExecAndParse)
}

func (t *Table) generateExecAndParse(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	execBytes, err := t.exec(ctx)
	if err != nil {
		// exec will error if there's no binary, so we never want to record that
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}

		// If the exec failed for some reason, it's probably better to return no results, and log the,
		// error. Returning an error here will cause a table failure, and thus break joins
		level.Info(t.logger).Log("msg", "failed to exec", "err", err)
		return nil, nil
	}

	data, err := t.dataParser.Parse(bytes.NewReader(execBytes))
	if err != nil {
		level.Info(t.logger).Log("msg", "failed to parse", "err", err)
		return nil, nil
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		// As we work with this, we might find that Flatten is missing some type handling. Bouncing through JSON is
		// a potential workaround
		flattened, err := dataflatten.Flatten(data, dataflatten.WithLogger(t.logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			level.Info(t.logger).Log("msg", "Error flattening data", "err", err)
			continue
		}
		results = append(results, ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}
