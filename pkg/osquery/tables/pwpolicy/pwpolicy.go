//+build darwin

// Package pwpolicy provides a table wrapper around the `pwpolicy` macOS
// command.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/pkg/dataflatten)

package pwpolicy

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const pwpolicyPath = "/usr/bin/pwpolicy"
const pwpolicyCmd = "getaccountpolicies"

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

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
		client:    client,
		logger:    logger,
		tableName: "kolide_pwpolicy",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, pwpolicyUsername := range tablehelpers.GetConstraints(queryContext, "username", tablehelpers.WithDefaults("")) {
		pwpolicyArgs := []string{pwpolicyCmd}

		if pwpolicyUsername != "" {
			pwpolicyArgs = append(pwpolicyArgs, "-u", pwpolicyUsername)
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("")) {
			pwPolicyOutput, err := t.execPwpolicy(ctx, pwpolicyArgs)
			if err != nil {
				level.Info(t.logger).Log("msg", "pwpolicy failed", "err", err)
				continue
			}

			flatData, err := t.flattenOutput(dataQuery, pwPolicyOutput)
			if err != nil {
				level.Info(t.logger).Log("msg", "flatten failed", "err", err)
				continue
			}

			for _, row := range flatData {
				p, k := row.ParentKey("/")

				res := map[string]string{
					"fullkey":  row.StringPath("/"),
					"parent":   p,
					"key":      k,
					"value":    row.Value,
					"query":    dataQuery,
					"username": pwpolicyUsername,
				}
				results = append(results, res)
			}
		}
	}

	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}

	return dataflatten.Plist(systemOutput, flattenOpts...)
}

func (t *Table) execPwpolicy(ctx context.Context, args []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pwpolicyPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling pwpolicy", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling pwpolicy. Got: %s", string(stderr.Bytes()))
	}

	// Remove first line of output because it always contains non-plist content
	outputBytes := bytes.SplitAfterN(stdout.Bytes(), []byte("\n"), 2)[1]

	return outputBytes, nil
}
