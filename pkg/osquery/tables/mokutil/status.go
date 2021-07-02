package mokutil

import (
	"bytes"
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

var mokutilLocations = []string{
	"/usr/bin/mokutil",
	"/usr/sbin/mokutil",
}

var (
	enabledBytes  = []byte("SecureBoot enabled")
	disabledBytes = []byte("SecureBoot disabled")
)

type Table struct {
	logger log.Logger
}

func StatusTablePlugin(_client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("secureboot"),
		table.TextColumn("_error"),
	}

	t := &Table{
		logger: logger,
	}

	return table.NewPlugin("kolide_mokutil_status", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	row := map[string]string{"secureboot": "unknown"}

	output, err := tablehelpers.Exec(ctx, t.logger, 2, mokutilLocations, []string{"--sb-state"})
	if err != nil {
		level.Info(t.logger).Log("msg", "mokutil failed", "err", err)
		row["_error"] = err.Error()
		return []map[string]string{row}, nil
	}

	switch {
	case bytes.HasPrefix(output, enabledBytes):
		row["secureboot"] = "enabled"
	case bytes.HasPrefix(output, disabledBytes):
		row["secureboot"] = "disabled"
	default:
		level.Info(t.logger).Log(
			"msg", "Can't parse mokutil output",
			output, string(output),
		)
		row["_error"] = fmt.Sprintf("Can't parse %s", string(output))
	}

	return []map[string]string{row}, nil
}
