package falconctl

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
	"github.com/osquery/osquery-go/plugin/table"
)

var (
	falconctlPaths = []string{"/opt/CrowdStrike/falconctl"}

	// allowedOptions is the list of options this table is allowed to query. Notable exceptions
	// are `systags` (which is parsed seperatedly) and `provisioning-token` (which is a secret).
	allowedOptions = []string{
		"--aid",
		"--aph",
		"--app",
		"--cid",
		"--feature",
		"--metadata-query",
		"--rfm-reason",
		"--rfm-state",
		"--tags",
		"--version",
	}

	defaultOption = strings.Join(allowedOptions, " ")
)

type execFunc func(context.Context, log.Logger, int, []string, []string) ([]byte, error)

type falconctlOptionsTable struct {
	logger    log.Logger
	tableName string
	execFunc  execFunc
}

func NewFalconctlOptionTable(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("options"),
	)

	t := &falconctlOptionsTable{
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
				level.Info(t.logger).Log("msg", "requested option not allowed", "option", option)
				// Consider this a fatal error, don't just go on.
				return nil, fmt.Errorf("requested option not allowed: %s", option)
			}
		}

		rowData := map[string]string{"options": requested}

		args := append([]string{"-g"}, options...)

		output, err := t.execFunc(ctx, t.logger, 30, falconctlPaths, args)
		if err != nil {
			level.Info(t.logger).Log("msg", "exec failed", "err", err)
			return nil, err
		}

		parsed, err := parseOptions(bytes.NewReader(output))
		if err != nil {
			level.Info(t.logger).Log("msg", "parse failed", "err", err)
			return nil, err
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithLogger(t.logger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Flatten(parsed, flattenOpts...)
			if err != nil {
				level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
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
