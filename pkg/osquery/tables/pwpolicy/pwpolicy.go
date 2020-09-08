//+build darwin

package pwpolicy

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client       *osquery.ExtensionManagerClient
	logger       log.Logger
	tableName    string
	execDataFunc func([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
}

const PwpolicyPath = "/usr/bin/pwpolicy"
const PwpolicyCmd = "getaccountpolicies"

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),
		table.TextColumn("username"),
	}

	t := &Table{
		client:       client,
		logger:       level.NewFilter(logger, level.AllowInfo()),
		tableName:    "kolide_pwpolicy",
		execDataFunc: dataflatten.Plist,
	}

	return table.NewPlugin(t.tableName, columns, t.generateExec)
}

func (t *Table) generateExec(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	var username string

	if q, ok := queryContext.Constraints["username"]; ok && len(q.Constraints) != 0 {
		username = q.Constraints[0].Expression
	}

	execBytes, err := t.exec(ctx, username)
	if err != nil {
		return results, errors.Wrap(err, "exec")
	}

	if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
		for _, constraint := range q.Constraints {
			dataQuery := constraint.Expression
			results = append(results, t.getRowsFromOutput(dataQuery, execBytes, username)...)
		}
	} else {
		results = append(results, t.getRowsFromOutput("", execBytes, username)...)
	}

	return results, nil
}

func (t *Table) exec(ctx context.Context, username string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{PwpolicyCmd}
	if username != "" {
		args = append(args, "-u")
		args = append(args, username)
	}

	cmd := exec.CommandContext(ctx, PwpolicyPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling %s", "args", PwpolicyPath, cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling %s. Got: %s", PwpolicyPath, string(stderr.Bytes()))
	}

	// Remove first line of output because it always contains non-plist content
	outputBytes := bytes.SplitAfterN(stdout.Bytes(), []byte("\n"), 2)[1]

	return outputBytes, nil
}

func (t *Table) getRowsFromOutput(dataQuery string, execOutput []byte, username string) []map[string]string {
	var results []map[string]string

	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	data, err := t.execDataFunc(execOutput, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
		return nil
	}

	for _, row := range data {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey":  row.StringPath("/"),
			"parent":   p,
			"key":      k,
			"value":    row.Value,
			"query":    dataQuery,
			"username": username,
		}
		results = append(results, res)
	}
	return results
}
