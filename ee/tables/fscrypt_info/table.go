package fscrypt_info

import (
	"log/slog"

	"github.com/osquery/osquery-go/plugin/table"
)

const (
	tableName = "kolide_fscrypt_info"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
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
	return table.NewPlugin(tableName, columns, t.generate)
}
