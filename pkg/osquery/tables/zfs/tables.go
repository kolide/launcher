package zfs

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const (
	zfsPath   = "/usr/sbin/zfs"
	zpoolPath = "/usr/sbin/zpool"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
	cmd    string
	args   []string
}

func columns() []table.ColumnDefinition {
	return []table.ColumnDefinition{
		table.TextColumn("name"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("source"),
	}
}

func ZfsPropertiesPlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	t := &Table{
		client: client,
		logger: logger,
		cmd:    zfsPath,
		args:   []string{"get", "-H", "all"},
	}

	return table.NewPlugin("kolide_zfs_properties", columns(), t.generate)
}

func ZpoolPropertiesPlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	t := &Table{
		client: client,
		logger: logger,
		cmd:    zpoolPath,
		args:   []string{"get", "-H", "all"},
	}

	return table.NewPlugin("kolide_zpool_properties", columns(), t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	output, err := tablehelpers.Exec(ctx, t.logger, 15, []string{t.cmd}, t.args)
	if err != nil {
		level.Info(t.logger).Log("msg", "failed to get zfs info", "err", err)
		// Don't error out if the binary isn't found
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}
		return nil, err
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
