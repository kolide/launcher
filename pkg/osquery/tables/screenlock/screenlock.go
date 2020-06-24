// +build darwin

// Screenlock calls out to osquery to get the screenlock status.
//
// While this could be implemented as a
// `dataflattentable.TablePluginExec` table, instead we have a
// dedicated table. This allows us to have a consistent set of
// columns, and change the implementation as desired. It's also
// simpler to add the `launchctl` functionality.
//
// Getting User Information
//
// This table uses undocumented APIs, There is some discussion at the
// PR adding the table. See
// https://github.com/osquery/osquery/pull/6243
//
// Empirically, it only works when run in the specific user
// context. Furthermore, setting the effective uid (as sudo might) is
// in adequate. Intead, we need to use `launchctl asuser`.
//
// Resulting data is odd. If a user is logged in, even inactive,
// correct data is returned. If a user has not ever configured these
// settings, the default values are returned. If the user has
// configured these settings, _and_ the user is not logged in, no data
// is returned.

package screenlock

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

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
		table.IntegerColumn("enabled"),
		table.IntegerColumn("grace_period"),

		table.TextColumn("user"),
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
		user := userConstraint.Expression

		osqueryResults, err := t.osqueryScreenlock(ctx, user)
		if err != nil {
			continue
		}

		for _, row := range osqueryResults {
			row["user"] = userConstraint.Expression
			results = append(results, row)
		}
	}
	return results, nil
}

func (t *Table) osqueryScreenlock(ctx context.Context, username string) ([]map[string]string, error) {

	targetUser, err := user.Lookup(username)
	if err != nil {
		return nil, errors.Wrapf(err, "looking up username %s", username)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"launchctl",
		"asuser",
		targetUser.Uid,
		t.osqueryd,
		"--config_path", "/dev/null",
		"--disable_events",
		"--disable_database",
		"--disable_audit",
		"--ephemeral",
		"-S",
		"--json",
		"select enabled, grace_period from screenlock",
	)

	dir, err := ioutil.TempDir("", "osq-screenlock")
	if err != nil {
		return nil, errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return nil, errors.Wrap(err, "chmod")
	}

	cmd.Dir = dir

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		level.Info(t.logger).Log(
			"msg", "Error getting screenlock status",
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(stdout.String()),
			"err", err,
		)
		return nil, errors.Wrap(err, "running osquery")
	}

	var osqueryResults []map[string]string

	if err := json.Unmarshal([]byte(stdout.Bytes()), &osqueryResults); err != nil {
		level.Info(t.logger).Log(
			"msg", "error unmarshalling json",
			"err", err,
			"stdout", stdout,
		)
		return nil, errors.Wrap(err, "unmarshalling json")
	}

	return osqueryResults, nil

}
