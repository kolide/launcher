//go:build darwin

// Package kextpolicy wraps github.com/knightsc/system_policy/sp to expose
// kernel extension policy data through our tablewrapper.
package kextpolicy

import (
	"context"
	"log/slog"

	"github.com/knightsc/system_policy/sp"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("developer_name"),
		table.TextColumn("application_name"),
		table.TextColumn("application_path"),
		table.TextColumn("team_id"),
		table.TextColumn("bundle_id"),
		table.IntegerColumn("allowed"),
		table.IntegerColumn("reboot_required"),
		table.IntegerColumn("modified"),
	}
	return tablewrapper.New(flags, slogger, "kext_policy", columns, generate,
		tablewrapper.WithDescription("macOS kernel extension (kext) approval policy, including developer name, team ID, bundle ID, and whether the extension is allowed. Useful for auditing which kernel extensions are approved or pending approval. Only relevant on Intel Macs running macOS 10.13+."),
	)
}

func generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	items := sp.CurrentKernelExtensionPolicy()
	for _, item := range items {
		row := map[string]string{
			"developer_name": item.DeveloperName,
			"team_id":        item.TeamID,
			"bundle_id":      item.BundleID,
		}
		if item.ApplicationName != "" {
			row["application_name"] = item.ApplicationName
		}
		if item.ApplicationPath != "" {
			row["application_path"] = item.ApplicationPath
		}
		if item.Allowed {
			row["allowed"] = "1"
		} else {
			row["allowed"] = "0"
		}
		if item.RebootRequired {
			row["reboot_required"] = "1"
		} else {
			row["reboot_required"] = "0"
		}
		if item.Modified {
			row["modified"] = "1"
		} else {
			row["modified"] = "0"
		}

		results = append(results, row)
	}

	return results, nil
}
