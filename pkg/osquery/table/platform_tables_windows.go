//go:build windows
// +build windows

package table

import (
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/dsim_default_associations"
	"github.com/kolide/launcher/ee/tables/execparsers/dsregcmd"
	"github.com/kolide/launcher/ee/tables/secedit"
	"github.com/kolide/launcher/ee/tables/wifi_networks"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/ee/tables/wmitable"

	"github.com/go-kit/kit/log"
	osquery "github.com/osquery/osquery-go"
)

func platformTables(logger log.Logger, currentOsquerydBinaryPath string) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		ProgramIcons(),
		dsim_default_associations.TablePlugin(logger),
		secedit.TablePlugin(logger),
		wifi_networks.TablePlugin(logger),
		windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, logger),
		windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, logger),
		wmitable.TablePlugin(logger),
		dataflattentable.NewExecAndParseTable(logger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
	}
}
