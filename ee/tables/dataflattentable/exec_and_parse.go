package dataflattentable

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type bytesFlattener interface {
	FlattenBytes([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
}

// execTableV2 is the next iteration of the dataflattentable wrapper. Aim to migrate exec based tables to this.
type execTableV2 struct {
	slogger        *slog.Logger
	tableName      string
	flattener      bytesFlattener
	timeoutSeconds int
	tabledebug     bool
	includeStderr  bool
	cmd            allowedcmd.AllowedCommand
	execArgs       []string
}

type execTableV2Opt func(*execTableV2)

func WithTimeoutSeconds(ts int) execTableV2Opt {
	return func(t *execTableV2) {
		t.timeoutSeconds = ts
	}
}

func WithTableDebug() execTableV2Opt {
	return func(t *execTableV2) {
		t.tabledebug = true
	}
}

func WithIncludeStderr() execTableV2Opt {
	return func(t *execTableV2) {
		t.includeStderr = true
	}
}

func NewExecAndParseTable(flags types.Flags, slogger *slog.Logger, tableName string, p parser, cmd allowedcmd.AllowedCommand, execArgs []string, opts ...execTableV2Opt) *table.Plugin {
	t := &execTableV2{
		slogger:        slogger.With("table", tableName),
		tableName:      tableName,
		flattener:      flattenerFromParser(p),
		timeoutSeconds: 30,
		cmd:            cmd,
		execArgs:       execArgs,
	}

	for _, opt := range opts {
		opt(t)
	}

	return tablewrapper.New(flags, slogger, t.tableName, Columns(), t.generate)
}

func (t *execTableV2) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	var results []map[string]string
	var stdout bytes.Buffer
	stdErr := io.Discard

	if t.includeStderr {
		stdErr = &stdout
	}

	if err := tablehelpers.Run(ctx, t.slogger, t.timeoutSeconds, t.cmd, t.execArgs, &stdout, stdErr); err != nil {
		// exec will error if there's no binary, so we never want to record that
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}
		observability.SetError(span, err)
		t.slogger.Log(ctx, slog.LevelInfo,
			"exec failed",
			"err", err,
		)
		return nil, nil
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithSlogger(t.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}
		if t.tabledebug {
			flattenOpts = append(flattenOpts, dataflatten.WithDebugLogging())
		}

		flattened, err := t.flattener.FlattenBytes(stdout.Bytes(), flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failure flattening output",
				"err", err,
			)
			continue
		}

		results = append(results, ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}
