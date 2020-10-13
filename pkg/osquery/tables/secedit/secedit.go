// +build windows

package secedit

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const seceditCmd = "secedit"

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("mergedpolicy"),
	)

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_secedit", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, mergedpolicy := range tablehelpers.GetConstraints(queryContext, "mergedpolicy", tablehelpers.WithDefaults("false")) {
		useMergedPolicy, err := strconv.ParseBool(mergedpolicy)
		if err != nil {
			level.Info(t.logger).Log("msg", "Cannot convert mergedpolicy constraint into a boolean value. Try passing \"true\"", "err", err)
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			secEditResults, err := t.execSecedit(ctx, useMergedPolicy)
			if err != nil {
				level.Info(t.logger).Log("msg", "secedit failed", "err", err)
				continue
			}

			flatData, err := t.flattenOutput(dataQuery, secEditResults)
			if err != nil {
				level.Info(t.logger).Log("msg", "flatten failed", "err", err)
				continue
			}

			rowData := map[string]string{
				"mergedpolicy": mergedpolicy,
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
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

	return dataflatten.Ini(systemOutput, flattenOpts...)
}

func (t *Table) execSecedit(ctx context.Context, mergedPolicy bool) ([]byte, error) {
	// The secedit.exe binary does not support outputting the data we need to stdout
	// Instead we create a tmp directory and pass it to secedit to write the data we need
	// in INI format.
	dir, err := ioutil.TempDir("", "kolide_secedit_config")
	if err != nil {
		return nil, errors.Wrap(err, "creating kolide_secedit_config tmp dir")
	}
	defer os.RemoveAll(dir)

	dst := filepath.Join(dir, "tmpfile.ini")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := []string{"/export", "/cfg", dst}
	if mergedPolicy {
		args = append(args, "/mergedpolicy")
	}

	cmd := exec.CommandContext(ctx, seceditCmd, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling secedit", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling secedit. Got: %s", stderr.String())
	}

	file, err := os.Open(dst)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening secedit output file: %s", dst)
	}
	defer file.Close()

	// By default, secedit outputs files encoded in UTF16 Little Endian. Sadly the Go INI parser
	// cannot read this format by default, therefore we decode the bytes into UTF-8
	rd := transform.NewReader(file, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading secedit output file: %s", err)
	}

	return data, nil
}
