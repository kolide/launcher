//go:build darwin

// Package legacyexec wraps github.com/knightsc/system_policy/sp to expose
// legacy (32-bit) execution history through our tablewrapper.
package legacyexec

import (
	"context"
	"log/slog"
	"time"

	"github.com/knightsc/system_policy/sp"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("exec_path"),
		table.TextColumn("mmap_path"),
		table.TextColumn("signing_id"),
		table.TextColumn("team_id"),
		table.TextColumn("cd_hash"),
		table.TextColumn("responsible_path"),
		table.TextColumn("developer_name"),
		table.TextColumn("last_seen"),
	}

	return tablewrapper.New(flags, slogger, "legacy_exec_history", columns, generate,
		tablewrapper.WithDescription("macOS legacy (32-bit) application execution history from the SystemPolicy framework, including executable paths, code signing info, and last seen timestamps. Useful for identifying legacy applications that have run on the system. Only relevant on macOS 10.14+."),
	)
}

func generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	items := sp.LegacyExecutionHistory()
	for _, item := range items {
		row := map[string]string{
			"exec_path": item.ExecPath,
			"last_seen": item.LastSeen.Format(time.RFC3339),
		}
		if item.MmapPath != "" {
			row["mmap_path"] = item.MmapPath
		}
		if item.SigningID != "" {
			row["signing_id"] = item.SigningID
		}
		if item.TeamID != "" {
			row["team_id"] = item.TeamID
		}
		if item.CDHash != "" {
			row["cd_hash"] = item.CDHash
		}
		if item.ResponsiblePath != "" {
			row["responsible_path"] = item.ResponsiblePath
		}
		if item.DeveloperName != "" {
			row["developer_name"] = item.DeveloperName
		}

		results = append(results, row)
	}

	return results, nil
}
