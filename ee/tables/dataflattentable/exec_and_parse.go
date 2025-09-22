package dataflattentable

import (
	"bytes"
	"context"
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
	slogger             *slog.Logger
	tableName           string
	flattener           bytesFlattener
	timeoutSeconds      int
	tabledebug          bool
	includeStderr       bool
	reportStderr        bool
	reportMissingBinary bool
	cmd                 allowedcmd.AllowedCommand
	execArgs            []string
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

// WithIncludeStderr combines stdout and stderr before attempting any parsing
func WithIncludeStderr() execTableV2Opt {
	return func(t *execTableV2) {
		t.includeStderr = true
	}
}

// WithReportStderr will include stderr (if populated) in the parsed output as a
// separate row in the results produced by the query
func WithReportStderr() execTableV2Opt {
	return func(t *execTableV2) {
		t.reportStderr = true
	}
}

// WithReportMissingBinary will include an error row in the results
// indicating that the binary is missing. Without this option, queries
// against missing binaries typically return no results, with the error
// being ignored.
// Note that for tables that run through macos RunDisclaimed, we cannot pass our missing
// binary errors back through- this information is conveyed through stderr, so callers
// should also include the WithReportStderr option to see the same behavior there.
func WithReportMissingBinary() execTableV2Opt {
	return func(t *execTableV2) {
		t.reportMissingBinary = true
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
	var stdout, stdErr bytes.Buffer

	// historically, callers expect that includeStderr implies stdout == stderr, so we do that here.
	// callers are free to ignore stdErr if not needed in other cases.
	if t.includeStderr {
		stdErr = stdout
	}

	if err := tablehelpers.Run(ctx, t.slogger, t.timeoutSeconds, t.cmd, t.execArgs, &stdout, &stdErr); err != nil {
		// exec will error if there's no binary, don't record that unless configured to do so
		if os.IsNotExist(errors.Cause(err)) || errors.Is(err, allowedcmd.ErrCommandNotFound) {
			if t.reportMissingBinary {
				return append(results, ToMap([]dataflatten.Row{
					{
						Path:  []string{"error"},
						Value: "binary is not present on device",
					},
				}, "*", nil)...), nil
			}

			return nil, nil
		}

		observability.SetError(span, err)
		t.slogger.Log(ctx, slog.LevelInfo,
			"exec failed",
			"err", err,
		)

		// Run failed, but we may have stderr to report with results anyway
		if t.reportStderr && stdErr.Len() > 0 {
			return append(results, ToMap([]dataflatten.Row{
				{
					Path:  []string{"error"},
					Value: stdErr.String(),
				},
			}, "*", nil)...), nil
		}

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

	// we could have made it through tablehelpers.Run above but still have seen error messaging
	// to stderr- ensure we include that here if configured to do so
	if t.reportStderr && stdErr.Len() > 0 {
		results = append(results, ToMap([]dataflatten.Row{
			{
				Path:  []string{"error"},
				Value: stdErr.String(),
			},
		}, "*", nil)...)
	}

	return results, nil
}
