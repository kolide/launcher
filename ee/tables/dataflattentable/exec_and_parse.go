package dataflattentable

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"text/template"

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
	description         string
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

// WithDescription provides a human-readable description of what the table returns and
// when it's useful. This is incorporated into the auto-generated table description that
// also includes the underlying command. For example:
//
//	WithDescription("information about disk partitions, volumes, and APFS containers on macOS. Useful for checking disk health or verifying APFS container structure")
func WithDescription(desc string) execTableV2Opt {
	return func(t *execTableV2) {
		t.description = desc
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
	tbl := &execTableV2{
		slogger:        slogger.With("table", tableName),
		tableName:      tableName,
		flattener:      flattenerFromParser(p),
		timeoutSeconds: 30,
		cmd:            cmd,
		execArgs:       execArgs,
	}

	for _, opt := range opts {
		opt(tbl)
	}

	return tablewrapper.New(
		flags,
		slogger,
		tbl.tableName,
		Columns(),
		tbl.generate,
		tablewrapper.WithDescription(tbl.Description()),
		tablewrapper.WithNote(EAVNote),
	)
}

const EAVNote = "This is an EAV (Entity-Attribute-Value) table. Rather than a column per field, data is returned as rows with fullkey, key, and value columns. fullkey is the slash-separated path to the value (e.g. network/interfaces/0/name), parent is the path of the containing object, and key is the leaf key name. Use the query constraint to filter by path -- it supports glob patterns (e.g. WHERE query = 'network/interfaces/*/name')."

var (
	defaultDescriptionTmpl = template.Must(template.New("default").Parse(
		"{{.TableName}} will exec the command `{{.Command}} {{.CommandArgs}}` and return the output."))
	richDescriptionTmpl = template.Must(template.New("rich").Parse(
		"{{.Description}}\n\nIt execs the command `{{.Command}} {{.CommandArgs}}`."))
)

// Description returns a string description suitable for inclusion in osquery spec files.
func (tbl *execTableV2) Description() string {
	cmdname := tbl.cmd.Name()
	cmdargs := tbl.execArgs
	// disclaimed commands are being exec'ed through launcher. So we need to untangle them
	if cmdname == "launcher" && len(cmdargs) > 0 && cmdargs[0] == "rundisclaimed" {
		if len(cmdargs) < 2 {
			cmdname = "~unknown~"
		} else {
			cmdname = cmdargs[1]
			cmdargs = cmdargs[2:]
		}
	}

	tmpl := defaultDescriptionTmpl
	if tbl.description != "" {
		tmpl = richDescriptionTmpl
	}

	templateVars := map[string]string{
		"TableName":   tbl.tableName,
		"Command":     cmdname,
		"CommandArgs": strings.Join(cmdargs, " "),
		"Description": tbl.description,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateVars); err != nil {
		return ""
	}
	return buf.String()
}

func (tbl *execTableV2) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", tbl.tableName)
	defer span.End()

	var results []map[string]string
	var stdout, stdErr bytes.Buffer

	// historically, callers expect that includeStderr implies stdout == stderr, so we do that here.
	// callers are free to ignore stdErr if not needed in other cases.
	if tbl.includeStderr {
		stdErr = stdout
	}

	if err := tablehelpers.Run(ctx, tbl.slogger, tbl.timeoutSeconds, tbl.cmd, tbl.execArgs, &stdout, &stdErr); err != nil {
		// exec will error if there's no binary, don't record that unless configured to do so
		if os.IsNotExist(errors.Cause(err)) || errors.Is(err, allowedcmd.ErrCommandNotFound) {
			if tbl.reportMissingBinary {
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
		tbl.slogger.Log(ctx, slog.LevelInfo,
			"exec failed",
			"err", err,
		)

		// Run failed, but we may have stderr to report with results anyway
		if tbl.reportStderr && stdErr.Len() > 0 {
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
			dataflatten.WithSlogger(tbl.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}
		if tbl.tabledebug {
			flattenOpts = append(flattenOpts, dataflatten.WithDebugLogging())
		}

		flattened, err := tbl.flattener.FlattenBytes(stdout.Bytes(), flattenOpts...)
		if err != nil {
			tbl.slogger.Log(ctx, slog.LevelInfo,
				"failure flattening output",
				"err", err,
			)

			continue
		}

		results = append(results, ToMap(flattened, dataQuery, nil)...)
	}

	// we could have made it through tablehelpers.Run above but still have seen error messaging
	// to stderr- ensure we include that here if configured to do so
	if tbl.reportStderr && stdErr.Len() > 0 {
		results = append(results, ToMap([]dataflatten.Row{
			{
				Path:  []string{"error"},
				Value: stdErr.String(),
			},
		}, "*", nil)...)
	}

	return results, nil
}
