// +build darwin

// Screenlock calls out to osquery to get the screenlock status.
//
// While this could be implemneted as a
// `dataflattentable.TablePluginExec` table, instead we have a
// dedicated table. This allows us to have a consistent set of
// columns, and changing the implementation as desired.

package screenlock

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kolide/launcher/pkg/cmdwrapper"

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

type osqueryScreenLockRow struct {
	Enabled     string `json:"enabled"`
	GracePeriod string `json:"grace_period"`
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger, osqueryd string) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("user"),
		table.IntegerColumn("enabled"),
		table.IntegerColumn("grace_period"),
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
				"select enabled, grace_period from screenlock",
			},
			cmdwrapper.RunAsUser(userConstraint.Expression),
		)

		if err != nil {
			level.Info(t.logger).Log(
				"msg", "Error getting screenlock status",
				"stderr", stderr,
				"stdout", stdout,
				"err", err,
			)
			continue
		}

		var osqueryResults []osqueryScreenLockRow

		if err := json.Unmarshal([]byte(stdout), &osqueryResults); err != nil {
			level.Info(t.logger).Log(
				"msg", "error unmarshalling json",
				"err", err,
				"stdout", stdout,
			)
			continue
		}

		for _, row := range osqueryResults {
			res := map[string]string{
				"user":         userConstraint.Expression,
				"enabled":      row.Enabled,
				"grace_period": row.GracePeriod,
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
