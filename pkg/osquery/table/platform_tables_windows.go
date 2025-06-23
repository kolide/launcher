//go:build windows
// +build windows

package table

import (
	"log/slog"

	json "github.com/kolide/launcher/ee/tables/execparsers/json"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/dsim_default_associations"
	"github.com/kolide/launcher/ee/tables/execparsers/dsregcmd"
	"github.com/kolide/launcher/ee/tables/secedit"
	"github.com/kolide/launcher/ee/tables/wifi_networks"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/ee/tables/wmitable"
	osquery "github.com/osquery/osquery-go"
)

func platformSpecificTables(k types.Knapsack, slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		ProgramIcons(k, slogger),
		dsim_default_associations.TablePlugin(k, slogger),
		secedit.TablePlugin(k, slogger),
		wifi_networks.TablePlugin(k, slogger),
		windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, k, slogger),
		windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, k, slogger),
		windowsupdatetable.CachedWindowsUpdatesTablePlugin(k, slogger, k.WindowsUpdatesCacheStore()),
		wmitable.TablePlugin(k, slogger),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
		dataflattentable.NewExecAndParseTable(k, slogger, "kolide_zscaler", json.Parser, allowedcmd.Zscli, []string{"status", "-s", "all"}, dataflattentable.WithReportStderr(), dataflattentable.WithReportMissingBinary()),
	}
}
