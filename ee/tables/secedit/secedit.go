//go:build windows
// +build windows

package secedit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("mergedpolicy"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_secedit"),
	}

	return tablewrapper.New(flags, slogger, "kolide_secedit", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_secedit")
	defer span.End()

	var results []map[string]string

	for _, mergedpolicy := range tablehelpers.GetConstraints(queryContext, "mergedpolicy", tablehelpers.WithDefaults("false")) {
		useMergedPolicy, err := strconv.ParseBool(mergedpolicy)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"cannot convert mergedpolicy constraint into a boolean value",
				"err", err,
			)
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			secEditResults, err := t.execSecedit(ctx, useMergedPolicy)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"secedit failed",
					"err", err,
				)
				continue
			}

			flatData, err := t.flattenOutput(dataQuery, secEditResults)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"flatten failed",
					"err", err,
				)
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
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	return dataflatten.Ini(systemOutput, flattenOpts...)
}

func (t *Table) execSecedit(ctx context.Context, mergedPolicy bool) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// The secedit.exe binary does not support outputting the data we need to stdout
	// Instead we create a tmp directory and pass it to secedit to write the data we need
	// in INI format.
	dir, err := agent.MkdirTemp("kolide_secedit_config")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_secedit_config tmp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	dst := filepath.Join(dir, "tmpfile.ini")

	args := []string{"/export", "/cfg", dst}
	if mergedPolicy {
		args = append(args, "/mergedpolicy")
	}

	var out bytes.Buffer
	if err := tablehelpers.Run(ctx, t.slogger, 30, allowedcmd.Secedit, args, &out, &out); err != nil {
		return nil, fmt.Errorf("calling secedit. Got: %s: %w", out.String(), err)
	}

	file, err := os.Open(dst)
	if err != nil {
		return nil, fmt.Errorf("error opening secedit output file: %s: %w", dst, err)
	}
	defer file.Close()

	// By default, secedit outputs files encoded in UTF16 Little Endian. Sadly the Go INI parser
	// cannot read this format by default, therefore we decode the bytes into UTF-8
	rd := transform.NewReader(file, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("error reading secedit output file: %s: %w", dst, err)
	}

	return data, nil
}
