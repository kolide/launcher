//go:build !windows
// +build !windows

package brew_upgradeable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("uid"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_brew_upgradeable"),
	}

	return tablewrapper.New(flags, slogger, "kolide_brew_upgradeable", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_brew_upgradeable")
	defer span.End()

	var results []map[string]string

	// Brew is owned by a single user on a system. Brew is only intended to run with the context of
	// that user. To reduce duplicating the WithUid table helper, we can find the owner of the binary,
	// and pass the said owner to the WIthUid method to handle setting the appropriate env vars.
	cmd, err := allowedcmd.Brew(ctx)

	if err != nil {
		if errors.Is(err, allowedcmd.ErrCommandNotFound) {
			// No data, no error
			return nil, nil
		}
		return nil, fmt.Errorf("failure allocating allowedcmd.Brew: %w", err)
	}

	info, err := os.Stat(cmd.Path)
	if err != nil {
		return nil, fmt.Errorf("failure getting FileInfo: %s. err: %w", cmd.Path, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("failure getting Sys data source: %s", cmd.Path)
	}

	uid := strconv.FormatUint(uint64(stat.Uid), 10)

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		// Brew can take a while to load the first time the command is ran, so leaving 60 seconds for the timeout here.
		var output bytes.Buffer
		var stderr bytes.Buffer

		if err := tablehelpers.Run(ctx, t.slogger, 60, allowedcmd.Brew, []string{"outdated", "--json"}, &output, &stderr, tablehelpers.WithUid(uid), tablehelpers.Disclaimed(ctx, "brew")); err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failure querying user brew installed packages",
				"err", err,
				"target_uid", uid,
				"output", output.String(),
				"stderr", stderr.String(),
			)
			continue
		}

		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithSlogger(t.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		flattened, err := dataflatten.Json(output.Bytes(), flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo, "failure flattening output", "err", err)
			continue
		}

		rowData := map[string]string{
			"uid": uid,
		}

		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}
