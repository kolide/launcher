//go:build darwin
// +build darwin

// Package ioreg provides a table wrapper around the `ioreg` macOS
// command.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/ee/dataflatten)
package ioreg

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Table struct {
	slogger   *slog.Logger
	tableName string
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		// ioreg input options. These match the ioreg
		// command line. See the ioreg man page.
		table.TextColumn("c"),
		table.IntegerColumn("d"),
		table.TextColumn("k"),
		table.TextColumn("n"),
		table.TextColumn("p"),
		table.IntegerColumn("r"), // boolean
	)

	t := &Table{
		slogger:   slogger.With("table", "kolide_ioreg"),
		tableName: "kolide_ioreg",
	}

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_ioreg")
	defer span.End()

	var results []map[string]string

	gcOpts := []tablehelpers.GetConstraintOpts{
		tablehelpers.WithDefaults(""),
		tablehelpers.WithAllowedCharacters(allowedCharacters),
		tablehelpers.WithSlogger(t.slogger),
	}

	for _, ioC := range tablehelpers.GetConstraints(queryContext, "c", gcOpts...) {
		// We always need "-a", it's the "archive" output
		ioregArgs := []string{"-a"}

		if ioC != "" {
			ioregArgs = append(ioregArgs, "-c", ioC)
		}

		for _, ioD := range tablehelpers.GetConstraints(queryContext, "d", gcOpts...) {
			if ioD != "" {
				ioregArgs = append(ioregArgs, "-d", ioD)
			}

			for _, ioK := range tablehelpers.GetConstraints(queryContext, "k", gcOpts...) {
				if ioK != "" {
					ioregArgs = append(ioregArgs, "-k", ioK)
				}
				for _, ioN := range tablehelpers.GetConstraints(queryContext, "n", gcOpts...) {
					if ioN != "" {
						ioregArgs = append(ioregArgs, "-n", ioN)
					}

					for _, ioP := range tablehelpers.GetConstraints(queryContext, "p", gcOpts...) {
						if ioP != "" {
							ioregArgs = append(ioregArgs, "-p", ioP)
						}

						for _, ioR := range tablehelpers.GetConstraints(queryContext, "r", gcOpts...) {
							switch ioR {
							case "", "0":
								// do nothing
							case "1":
								ioregArgs = append(ioregArgs, "-r")
							default:
								t.slogger.Log(ctx, slog.LevelInfo,
									"r should be blank, 0, or 1",
									"r_value", ioR,
								)
								continue
							}

							for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
								// Finally, an inner loop

								ioregOutput, err := tablehelpers.RunSimple(ctx, t.slogger, 30, allowedcmd.Ioreg, ioregArgs)
								if err != nil {
									t.slogger.Log(ctx, slog.LevelInfo,
										"ioreg failed",
										"err", err,
									)
									continue
								}

								flatData, err := t.flattenOutput(dataQuery, ioregOutput)
								if err != nil {
									t.slogger.Log(ctx, slog.LevelInfo,
										"flatten failed",
										"err", err,
									)
									continue
								}

								rowData := map[string]string{
									"c": ioC,
									"d": ioD,
									"k": ioK,
									"n": ioN,
									"p": ioP,
									"r": ioR,
								}

								results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
							}
						}
					}
				}
			}
		}
	}

	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	return dataflatten.Plist(systemOutput, flattenOpts...)
}
