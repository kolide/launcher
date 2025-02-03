//go:build darwin
// +build darwin

// Package osquery_exec_table provides a table generator that will
// call osquery in a user context.
//
// This is necessary because some macOS tables need to run in user
// context. Running this in root context returns no
// results. Furthermore, these cannot run in sudo. Sudo sets the
// effective uid, but instead we need a bunch of keychain context.
//
// Resulting data is odd. If a user is logged in, even inactive,
// correct data is returned. If a user has not ever configured these
// settings, the default values are returned. If the user has
// configured these settings, _and_ the user is not logged in, no data
// is returned.

package osquery_user_exec_table

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	allowedUsernameCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. "
)

type Table struct {
	slogger   *slog.Logger
	osqueryd  string
	query     string
	tablename string
}

func TablePlugin(
	flags types.Flags, slogger *slog.Logger, tablename string, osqueryd string,
	osqueryQuery string, columns []table.ColumnDefinition,
) *table.Plugin {
	columns = append(columns, table.TextColumn("user"))

	t := &Table{
		slogger:   slogger.With("table", tablename),
		osqueryd:  osqueryd,
		query:     osqueryQuery,
		tablename: tablename,
	}

	return tablewrapper.New(flags, slogger, t.tablename, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", t.tablename)
	defer span.End()

	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "user",
		tablehelpers.WithAllowedCharacters(allowedUsernameCharacters),
	)

	if len(users) == 0 {
		return nil, fmt.Errorf("The %s table requires a user", t.tablename)
	}

	for _, user := range users {
		osqueryResults, err := tablehelpers.ExecOsqueryLaunchctlParsed(ctx, t.slogger, 5, user, t.osqueryd, t.query)
		if err != nil {
			continue
		}

		for _, row := range osqueryResults {
			row["user"] = user
			results = append(results, row)
		}
	}
	return results, nil
}
