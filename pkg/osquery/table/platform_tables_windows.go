//go:build windows
// +build windows

package table

import (
	"log/slog"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/dsim_default_associations"
	"github.com/kolide/launcher/ee/tables/execparsers/dsregcmd"
	"github.com/kolide/launcher/ee/tables/ntfs_ads_zone_id/zone_identifier"
	"github.com/kolide/launcher/ee/tables/secedit"
	"github.com/kolide/launcher/ee/tables/wifi_networks"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/ee/tables/wmitable"
	osquery "github.com/osquery/osquery-go"
)

func platformSpecificTables(slogger *slog.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		ProgramIcons(),
		dsim_default_associations.TablePlugin(slogger),
		zone_identifier.TablePlugin(slogger),
		secedit.TablePlugin(slogger),
		wifi_networks.TablePlugin(slogger),
		windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, slogger),
		windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, slogger),
		wmitable.TablePlugin(slogger),
		dataflattentable.NewExecAndParseTable(slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
	}
}
