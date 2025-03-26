package fscrypt_info

import (
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	tableName = "kolide_fscrypt_info"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.IntegerColumn("encrypted"),
		table.TextColumn("mountpoint"),
		table.TextColumn("locked"),
		table.TextColumn("filesystem_type"),
		table.TextColumn("device"),
		table.TextColumn("contents_algo"),
		table.TextColumn("filename_algo"),
	}

	t := &Table{
		slogger: slogger.With("table", tableName),
	}
	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}
