//go:build windows
// +build windows

package table

import (
	"log/slog"

	"github.com/go-kit/kit/log"
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

func platformSpecificTables(slogger *slog.Logger, logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		ProgramIcons(),
		dsim_default_associations.TablePlugin(slogger, logger),
		secedit.TablePlugin(slogger, logger),
		wifi_networks.TablePlugin(slogger, logger),
		windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, slogger, logger),
		windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, slogger, logger),
		wmitable.TablePlugin(slogger, logger),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
	}
}
