package dataflattentable

import (
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

type flattenBytesInt interface {
	FlattenBytes([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
}

// execTable is the next iteration of the dataflattentable wrapper. Aim to migrate exec to this.
type execTableV2 struct {
	logger         log.Logger
	tableName      string
	flattener      flattenBytesInt
	timeoutSeconds int
	execPaths      []string
	execArgs       []string
}

type execTableV2Opt func(*execTableV2)

func WithTimeout(ts int) execTableV2Opt {
	return func(t *execTableV2) {
		t.timeoutSeconds = ts
	}
}

func WithAdditionalExecPaths(paths ...string) execTableV2Opt {
	return func(t *execTableV2) {
		t.execPaths = append(t.execPaths, paths...)
	}
}

func NewExecAndParseTable(logger log.Logger, tableName string, parser parserInt, execCmd []string, opts ...execTableV2Opt) *table.Plugin {
	t := &execTableV2{
		logger:         level.NewFilter(log.With(logger, "table", tableName), level.AllowInfo()),
		tableName:      tableName,
		flattener:      flattenerFromParser(parser),
		timeoutSeconds: 30,
		execPaths:      execCmd[:1],
		execArgs:       execCmd[1:],
	}

	for _, opt := range opts {
		opt(t)
	}

	return table.NewPlugin(t.tableName, Columns(), t.generate)
}

func (t *execTableV2) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	execOutput, err := tablehelpers.Exec(ctx, t.logger, t.timeoutSeconds, t.execPaths, t.execArgs)
	if err != nil {
		// exec will error if there's no binary, so we never want to record that
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}
		level.Info(t.logger).Log("msg", "exec failed", "err", err)
		return nil, nil
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithLogger(t.logger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		flattened, err := t.flattener.FlattenBytes(execOutput, flattenOpts...)
		if err != nil {
			level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
			continue
		}

		results = append(results, ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}
