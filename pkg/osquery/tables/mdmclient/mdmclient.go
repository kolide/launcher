// +build !windows
// (skip building windows, since the newline replacement doesn't work there)

// Package mdmclient provides a table that parses the mdmclient
// output. Empirically, this seems to be an almost gnustep
// plist. With some headers. So, unwind that.

package mdmclient

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const mdmclientPath = "/usr/libexec/mdmclient"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

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
		tableName: "kolide_mdmclient",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	gcOpts := []tablehelpers.GetConstraintOpts{
		tablehelpers.WithAllowedCharacters(allowedCharacters),
		tablehelpers.WithLogger(t.logger),
		tablehelpers.WithDefaults(""),
	}

	for _, mdmclientCommand := range tablehelpers.GetConstraints(queryContext, "command", gcOpts...) {
		if mdmclientCommand == "" {
			level.Info(t.logger).Log("msg", "command must not be blank")
			continue
		}

		if !strings.HasPrefix(mdmclientCommand, "Query") {
			level.Info(t.logger).Log("msg", "Only Query commands are supported")
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

			mdmclientOutput, err := t.execMdmclient(ctx, mdmclientCommand)
			if err != nil {
				level.Info(t.logger).Log("msg", "mdmclient failed", "err", err)
				continue
			}

			flatData, err := t.flattenOutput(dataQuery, mdmclientOutput)
			if err != nil {
				level.Info(t.logger).Log("msg", "flatten failed", "err", err)
				continue
			}

			rowData := map[string]string{
				"command": mdmclientCommand,
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}
	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	converted, err := t.transformOutput(systemOutput)
	if err != nil {
		level.Info(t.logger).Log("msg", "converting mdmclient output", "err", err)
		return nil, errors.Wrap(err, "converting")
	}

	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}

	return dataflatten.Plist(converted, flattenOpts...)
}

func (t *Table) execMdmclient(ctx context.Context, command string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, mdmclientPath, command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling mdmclient", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling mdmclient. Got: %s", string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}

// transformOutput has some hackish rules to transform the output into a "proper" gnustep plist
func (t *Table) transformOutput(in []byte) ([]byte, error) {
	out := bytes.Replace(in, []byte("Daemon response: {"), []byte("DaemonResponse = {"), 1)
	out = bytes.Replace(out, []byte("Agent response: {"), []byte("AgentResponse = {"), 1)

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
