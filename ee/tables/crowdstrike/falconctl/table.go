//go:build !windows
// +build !windows

package falconctl

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

var (
	// allowedOptions is the list of options this table is allowed to query. Notable exceptions
	// are `systags` (which is parsed seperatedly) and `provisioning-token` (which is a secret).
	allowedOptions = []string{
		"--aid",
		"--apd",
		"--aph",
		"--app",
		"--cid",
		"--feature",
		"--metadata-query",
		"--rfm-reason",
		"--rfm-state",
		"--rfm-history",
		"--tags",
		"--version",
	}

	defaultOption = strings.Join(allowedOptions, " ")
)

type execFunc func(context.Context, log.Logger, int, allowedcmd.AllowedCommand, []string, bool) ([]byte, error)

type falconctlOptionsTable struct {
	slogger   *slog.Logger
	logger    log.Logger // preserved temporarily for use in dataflattentables/tablehelpers
	tableName string
	execFunc  execFunc
}

func NewFalconctlOptionTable(slogger *slog.Logger, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("options"),
	)

	t := &falconctlOptionsTable{
		slogger:   slogger.With("table", "kolide_falconctl_options"),
		logger:    log.With(logger, "table", "kolide_falconctl_options"),
		tableName: "kolide_falconctl_options",
		execFunc:  tablehelpers.Exec,
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *falconctlOptionsTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// Note that we don't use tablehelpers.AllowedValues here, because that would disallow us from
	// passing `where options = "--aid --aph"`, and allowing that, allows us a single exec.
OUTER:
	for _, requested := range tablehelpers.GetConstraints(
		queryContext,
		"options",
		tablehelpers.WithDefaults(defaultOption),
	) {

		options := strings.Split(requested, " ")

		// Check that all requested options are allowed
		for _, option := range options {
			option = strings.Trim(option, " ")
			if !optionAllowed(option) {
				t.slogger.Log(ctx, slog.LevelInfo,
					"requested option not allowed",
					"option", option,
				)
				continue OUTER
			}
		}

		rowData := map[string]string{"options": requested}

		// As I understand it the falconctl command line uses `-g` to indicate it's fetching the options settings, and
		// then the list of options to fetch. Set the command line thusly.
		args := append([]string{"-g"}, options...)

		output, err := t.execFunc(ctx, t.logger, 30, allowedcmd.Falconctl, args, false)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"exec failed",
				"err", err,
			)
			synthesizedData := map[string]string{
				"_error": fmt.Sprintf("falconctl parse failure: %s", err),
			}

			flattened, err := dataflatten.Flatten(synthesizedData)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failure flattening output",
					"err", err,
				)
				continue
			}

			results = append(results, dataflattentable.ToMap(flattened, "", rowData)...)
			continue
		}

		parsed, err := parseOptions(bytes.NewReader(output))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"parse failed",
				"err", err,
			)
			parsed = map[string]string{
				"_error": fmt.Sprintf("falconctl parse failure: %s", err),
			}
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithLogger(t.logger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Flatten(parsed, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failure flattening output",
					"err", err,
				)
				continue
			}

			results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
		}
	}

	return results, nil

}

func optionAllowed(opt string) bool {
	for _, b := range allowedOptions {
		if b == opt {
			return true
		}
	}
	return false
}
