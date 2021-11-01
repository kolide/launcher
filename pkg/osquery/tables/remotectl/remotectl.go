// +build !windows
// (skip building windows, since the newline replacement doesn't work there)

// Package remotectl provides a table that parses the remotectl
// output. Empirically, this seems to be an almost gnustep
// plist. With some headers. So, unwind that.

package remotectl

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const (
	remotectlCommand = "dumpstate"
	remotectlPath    = "/usr/libexec/remotectl"
)

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("command"),
	)

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_remotectl",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// Used for dataflatten query column separate from exec pattern
	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

		remotectlOutput, err := tablehelpers.Exec(ctx, t.logger, 30, []string{remotectlPath}, []string{remotectlCommand})
		if err != nil {
			level.Info(t.logger).Log("msg", "remotectl failed", "err", err)
			continue
		}

		flatData, err := t.flattenOutput(dataQuery, remotectlOutput)
		if err != nil {
			level.Info(t.logger).Log("msg", "flatten failed", "err", err)
			continue
		}

		rowData := map[string]string{
			"command": remotectlCommand,
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}
	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	// convert mangled output
	converted, err := t.transformOutput(systemOutput)
	if err != nil {
		level.Info(t.logger).Log("msg", "converting remotectl output", "err", err)
		return nil, errors.Wrap(err, "converting")
	}
	fmt.Println(string(converted))

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	return dataflatten.Plist(converted, flattenOpts...)
}

// transformOutput has some hackish rules to transform the output into a "proper" gnustep plist
func (t *Table) transformOutput(in []byte) ([]byte, error) {
	//
	out := bytes.Replace(in, []byte("Local device"), []byte("LocalDevice = {"), 1)
	out = bytes.Replace(out, []byte("Found localbridge (bridge)"), []byte("LocalBridge = {"), 1)

	// Fritz: need to set a delimiter of key:value that is: '=>' instead of '='
	// and deal with weird ":" newline shenanigans, generally this needs less
	// naive and more elegant transformation
	out = bytes.ReplaceAll(out, []byte(" => "), []byte(" = "))
	out = bytes.ReplaceAll(out, []byte(": "), []byte(" = "))
	out = bytes.ReplaceAll(out, []byte(":\n"), []byte("}\n"))
	out = bytes.ReplaceAll(out, []byte("\n"), []byte(";\n"))

	// This would, honestly, be cleaner as a regex. The \n aren't
	// quite right. We want to replace any unindented } with a
	// };. Which is just a hack, because we really want to replace
	// the one that matches the response structures.
	out = bytes.Replace(out, []byte("\n}\n"), []byte("\n};\n"), 2)

	var retOut []byte
	retOut = append(retOut, "{\n"...)
	retOut = append(retOut, out...)
	retOut = append(retOut, "\n}\n"...)
	return retOut, nil
}
