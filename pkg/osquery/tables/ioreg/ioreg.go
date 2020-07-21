//+build darwin

// Package ioreg provides a tablle wrapper around the `ioreg` macOS
// command.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/pkg/dataflatten)

package ioreg

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

const ioregPath = "/usr/sbin/ioreg"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

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

		// ioreg input options. These match the ioreg
		// command line. See the ioreg man page.
		table.TextColumn("c"),
		table.IntegerColumn("d"),
		table.TextColumn("k"),
		table.TextColumn("n"),
		table.TextColumn("p"),
		table.IntegerColumn("r"), // boolean
	}

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_ioreg",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	gcOpts := []tablehelpers.GetConstraintOpts{
		tablehelpers.WithDefaults(""),
		tablehelpers.WithAllowedCharacters(allowedCharacters),
		tablehelpers.WithLogger(t.logger),
	}

	for _, ioC := range tablehelpers.GetConstraints(queryContext, "c", gcOpts...) {
		ioregArgs := []string{}

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
								level.Info(t.logger).Log("msg", "r should be blank, 0, or 1")
								continue
							}

							for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("")) {
								// Finally, an inner loop

								ioregOutput, err := t.execIoreg(ctx, ioregArgs)
								if err != nil {
									level.Info(t.logger).Log("msg", "ioreg failed", "err", err)
									continue
								}

								flatData, err := t.flattenOutput(dataQuery, ioregOutput)
								if err != nil {
									level.Info(t.logger).Log("msg", "flatten failed", "err", err)
									continue
								}

								for _, row := range flatData {
									p, k := row.ParentKey("/")

									res := map[string]string{
										"fullkey": row.StringPath("/"),
										"parent":  p,
										"key":     k,
										"value":   row.Value,
										"query":   dataQuery,
										"c":       ioC,
										"d":       ioD,
										"k":       ioK,
										"n":       ioN,
										"p":       ioP,
										"r":       ioR,
									}
									results = append(results, res)
								}
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

func (t *Table) execIoreg(ctx context.Context, args []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args = append(args, "-a")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ioregPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling ioreg", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling ioreg. Got: %s", string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}
