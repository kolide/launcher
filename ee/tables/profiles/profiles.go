//go:build darwin
// +build darwin

// Package profiles provides a table wrapper around the various
// profiles options.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/ee/dataflatten)
package profiles

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	userAllowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	typeAllowedCharacters = "abcdefghijklmnopqrstuvwxyz"
)

var (
	allowedCommands = []string{"show", "list", "status"} // Consider "sync" but that's a write comand
)

type Table struct {
	slogger   *slog.Logger
	tableName string
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	// profiles options. See `man profiles`. These may not be needed,
	// we use `show -all` as the default, and it probably covers
	// everything.
	columns := dataflattentable.Columns(
		table.TextColumn("user"),
		table.TextColumn("command"),
		table.TextColumn("type"),
	)

	t := &Table{
		slogger:   slogger.With("table", "kolide_profiles"),
		tableName: "kolide_profiles",
	}

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	var results []map[string]string

	for _, command := range tablehelpers.GetConstraints(queryContext, "command", tablehelpers.WithAllowedValues(allowedCommands), tablehelpers.WithDefaults("show")) {
		for _, profileType := range tablehelpers.GetConstraints(queryContext, "type", tablehelpers.WithAllowedCharacters(typeAllowedCharacters), tablehelpers.WithDefaults("")) {
			for _, user := range tablehelpers.GetConstraints(queryContext, "user", tablehelpers.WithAllowedCharacters(userAllowedCharacters), tablehelpers.WithDefaults("_all")) {
				for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

					rowData, err := t.generateProfile(ctx, command, profileType, user, dataQuery)
					if err != nil {
						t.slogger.Log(ctx, slog.LevelWarn,
							"generating profile",
							"command", command,
							"profile_type", profileType,
							"user", user,
							"data_query", dataQuery,
							"err", err,
						)
						continue
					}

					if len(rowData) > 0 {
						results = append(results, rowData...)
					}
				}
			}
		}
	}
	return results, nil
}

func (t *Table) generateProfile(ctx context.Context, command string, profileType string, user string, dataQuery string) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// apple documents `-output stdout-xml` as sending the
	// output to stdout, in xml. This, however, does not work
	// for some subset of the profiles command. I've reported it
	// to apple (feedback FB8962811), and while it may someday
	// be fixed, we need to support it where it is.
	dir, err := agent.MkdirTemp("kolide_profiles")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_profiles tmp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	outputFile := filepath.Join(dir, "output.xml")

	profileArgs := []string{command, "-output", outputFile}

	if profileType != "" {
		profileArgs = append(profileArgs, "-type", profileType)
	}

	// setup the command line. This table overloads the `user`
	// column so one can select either:
	//   * All profiles merged, using the special value `_all` (this is the default)
	//   * The device profiles, using the special value `_device`
	//   * a user specific one, using the username
	switch {
	case user == "" || user == "_all":
		profileArgs = append(profileArgs, "-all")
	case user == "_device":
		break
	case user != "":
		profileArgs = append(profileArgs, "-user", user)
	default:
		return nil, fmt.Errorf("Unknown user argument: %s", user)
	}

	output, err := tablehelpers.RunSimple(ctx, t.slogger, 30, allowedcmd.Profiles, profileArgs)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"ioreg exec failed",
			"err", err,
		)
		return nil, nil
	}

	if bytes.Contains(output, []byte("requires root privileges")) {
		t.slogger.Log(ctx, slog.LevelInfo,
			"ioreg requires root privileges",
		)
		return nil, nil
	}

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	flatData, err := dataflatten.PlistFile(outputFile, flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"flatten failed",
			"err", err,
		)
		return nil, nil
	}

	rowData := map[string]string{
		"command": command,
		"type":    profileType,
		"user":    user,
	}

	return dataflattentable.ToMap(flatData, dataQuery, rowData), nil
}
