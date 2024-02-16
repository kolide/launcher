//go:build darwin
// +build darwin

package filevault

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("status"),
	}

	t := &Table{
		slogger: slogger.With("table", "kolide_filevault"),
	}

	return table.NewPlugin("kolide_filevault", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	output, err := tablehelpers.Exec(ctx, t.slogger, 10, allowedcmd.Fdesetup, []string{"status"}, false)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"fdesetup failed",
			"err", err,
		)

		// Don't error out if the binary isn't found
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}
		return nil, fmt.Errorf("calling fdesetup: %w", err)
	}

	status := strings.TrimSuffix(string(output), "\n")

	// It's a bit verbose to instatiate this directly, but it
	// seems better than a needless append.
	results := []map[string]string{
		{
			"status": status,
		},
	}
	return results, nil
}
