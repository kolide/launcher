// +build darwin

package filevault

import (
	"context"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const fdesetupPath = "/usr/bin/fdesetup"

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("status"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_filevault", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	output, err := tablehelpers.Exec(ctx, t.logger, 10, []string{fdesetupPath}, []string{"status"})
	if err != nil {
		level.Info(t.logger).Log("msg", "fdesetup failed", "err", err)
		return nil, errors.Wrap(err, "calling fdesetup")
	}

	status := strings.TrimSuffix(string(output), "\n")

	// It's a bit verbose to instatiate this directly, but it
	// seems better than a needless append.
	results := []map[string]string{
		map[string]string{
			"status": status,
		},
	}
	return results, nil
}
