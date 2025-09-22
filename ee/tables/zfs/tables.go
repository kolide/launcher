//go:build !windows
// +build !windows

package zfs

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"

	"os"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.@/"

type Table struct {
	slogger   *slog.Logger
	cmd       allowedcmd.AllowedCommand
	tableName string
}

func columns() []table.ColumnDefinition {
	return []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("source"),
	}
}

func ZfsPropertiesPlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	t := &Table{
		slogger:   slogger.With("table", "kolide_zfs_properties"),
		cmd:       allowedcmd.Zfs,
		tableName: "kolide_zfs_properties",
	}

	return tablewrapper.New(flags, slogger, "kolide_zfs_properties", columns(), t.generate)
}

func ZpoolPropertiesPlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	t := &Table{
		slogger:   slogger.With("table", "kolide_zpool_properties"),
		cmd:       allowedcmd.Zpool,
		tableName: "kolide_zpool_properties",
	}

	return tablewrapper.New(flags, slogger, "kolide_zpool_properties", columns(), t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	// Generate ZFS commands.
	//
	// keys are comma separated. Default to `all` to get everything
	// names are args. Default to none to get everything
	//
	// These commands all work:
	// zfs get -H encryption
	// zfs get -H encryption tank-enc/home-sephenc tank-clear/ds-enc
	// zfs get -H all tank-enc/home-sephenc tank-clear/ds-enc

	keys := tablehelpers.GetConstraints(queryContext, "key", tablehelpers.WithDefaults("all"), tablehelpers.WithAllowedCharacters(allowedCharacters))
	names := tablehelpers.GetConstraints(queryContext, "name", tablehelpers.WithAllowedCharacters(allowedCharacters))

	args := []string{
		"get",
		"-H", strings.Join(keys, ","),
	}

	args = append(args, names...)

	output, err := tablehelpers.RunSimple(ctx, t.slogger, 15, t.cmd, args)
	if err != nil {
		// exec will error if there's no binary, so we never want to record that
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}

		// ZFS can fail for weird reasons. I've started seeing fedora
		// machine that ship a zfs userspace, but no kernel driver. So,
		// only log, don't return the errors.
		t.slogger.Log(ctx, slog.LevelInfo,
			"failed to get zfs info",
			"err", err,
		)
		return nil, nil
	}

	return parseColumns(output)
}

// parseColumns parses the zfs property output. It conveniently comes
// in in a very simple format, already EAV style.
func parseColumns(rawdata []byte) ([]map[string]string, error) {
	data := []map[string]string{}

	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 4)
		row := map[string]string{
			"name":   parts[0],
			"key":    parts[1],
			"value":  parts[2],
			"source": parts[3],
		}
		data = append(data, row)
	}

	return data, nil
}
