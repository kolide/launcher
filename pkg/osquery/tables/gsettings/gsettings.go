package gsettings

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const gsettingsPath = "/usr/bin/gsettings"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."

type GsettingsValues struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes func(ctx context.Context, buf *bytes.Buffer) error
}

// Settings returns a table plugin for querying setting values from the
// gsettings command.
func Settings(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	var columns []table.ColumnDefinition

	// we don't need the 'query' column.
	// we could also just type out all the cols we do want
	for _, col := range dataflattentable.Columns(table.TextColumn("schema")) {
		if col.Name != "query" {
			columns = append(columns, col)
		}
	}

	t := &GsettingsValues{
		client:   client,
		logger:   logger,
		getBytes: execGsettings,
	}

	return table.NewPlugin("kolide_gsettings", columns, t.generate)
}

func (t *GsettingsValues) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	var output bytes.Buffer

	err := t.getBytes(ctx, &output)
	if err != nil {
		level.Info(t.logger).Log("msg", "gsettings failed", "err", err)

		return results, err
	}
	data, err := t.flatten(&output)
	if err != nil {
		level.Info(t.logger).Log(
			"msg", "error flattening data",
			"err", err,
		)
		return results, err
	}

	for _, row := range data {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey": row.StringPath("/"),
			"parent":  p,
			"key":     k,
			"value":   row.Value,
			"schema":  p,
		}
		results = append(results, res)
	}

	return results, nil
}

// execGsettings writes the output of running 'gsettings' command into the supplied bytes buffer
func execGsettings(ctx context.Context, buf *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "list-recursively")
	dir, err := ioutil.TempDir("", "osq-gsettings")
	if err != nil {
		return errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return errors.Wrap(err, "chmod")
	}
	cmd.Dir = dir
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "running osquery")
	}

	return nil
}

func (t *GsettingsValues) flatten(buffer *bytes.Buffer) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{}
	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}
	results := t.parse(buffer)
	var rows []dataflatten.Row

	for _, result := range results {
		row := dataflatten.NewRow([]string{result["schema"], result["key"]}, result["value"])
		rows = append(rows, row)
	}
	return rows, nil
}

func (t *GsettingsValues) parse(input *bytes.Buffer) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		row := make(map[string]string)
		parts := strings.SplitN(line, " ", 3)
		row["schema"] = parts[0]
		row["key"] = parts[1]
		row["value"] = parts[2]
		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results
}
