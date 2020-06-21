// +build darwin

// Screenlock calls out to osquery to get the screenlock status.
//
// Implemented as it's own table, and not a
// `dataflattentable.TablePluginExec` call, because of the `user`
// constraint. May change at any time.

package screenlock

import (
	"context"
	"strings"

	"github.com/kolide/launcher/pkg/cmdwrapper"
	"github.com/kolide/launcher/pkg/dataflatten"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	osqueryd string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger, osqueryd string) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("user"),
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	t := &Table{
		client:   client,
		logger:   logger,
		osqueryd: osqueryd,
	}

	return table.NewPlugin("kolide_screenlock", columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	userQ, ok := queryContext.Constraints["user"]
	if !ok || len(userQ.Constraints) == 0 {
		return nil, errors.New("The kolide_screenlock table requires a user")
	}

	for _, userConstraint := range userQ.Constraints {
		if !onlyAllowedCharacters(userConstraint.Expression) {
			level.Info(t.logger).Log("msg", "Disallowed character in user expression")
			return nil, errors.New("Disallowed character in user expression")
		}

		stdout, stderr, err := cmdwrapper.Run(
			ctx,
			t.osqueryd,
			[]string{
				"--config_path", "/dev/null",
				"-S",
				"--json",
				"select * from screenlock",
			},
			cmdwrapper.RunAsUser(userConstraint.Expression),
		)

		if err != nil {
			level.Info(t.logger).Log(
				"msg", "Error getting screenlock status",
				"stderr", stderr,
				"stdout", stdout,
			)
			continue
		}

		flattenOpts := []dataflatten.FlattenOpts{}
		if t.logger != nil {
			flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
		}

		data, err := dataflatten.Json([]byte(stdout), flattenOpts...)
		if err != nil {
			level.Info(t.logger).Log("msg", "failure flattening", "err", err)
			continue
		}

		for _, row := range data {
			p, k := row.ParentKey("/")

			res := map[string]string{
				"user":    userConstraint.Expression,
				"fullkey": row.StringPath("/"),
				"parent":  p,
				"key":     k,
				"value":   row.Value,
			}
			results = append(results, res)
		}
	}
	return results, nil
}

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"

func onlyAllowedCharacters(input string) bool {
	for _, char := range input {
		if !strings.ContainsRune(allowedCharacters, char) {
			return false
		}
	}
	return true
}
