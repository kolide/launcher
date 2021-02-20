// +build windows

package table

import (
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/secedit"
	"github.com/kolide/launcher/pkg/osquery/tables/wifi_networks"
	"github.com/kolide/launcher/pkg/osquery/tables/windows_associations"
	"github.com/kolide/launcher/pkg/osquery/tables/wmitable"

	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

const (
	wuUpdatesPowershell = `
$WUSession = New-Object -ComObject Microsoft.Update.Session
$WUSearcher = $WUSession.CreateUpdateSearcher()
$UpdateCollection = $WUSearcher.Search("Type='Software'")
$UpdateCollection.Updates | ConvertTo-Json
`
	wuHistoryPowershell = `
$WUSession = New-Object -ComObject Microsoft.Update.Session
$WUSearcher = $WUSession.CreateUpdateSearcher()
$WUSearcher.QueryHistory(0, $WUSearcher.GetTotalHistoryCount()) | ConvertTo-Json
`
)

func platformTables(client *osquery.ExtensionManagerClient, logger log.Logger, currentOsquerydBinaryPath string) []*table.Plugin {
	return []*table.Plugin{
		ProgramIcons(),
		secedit.TablePlugin(client, logger),
		wifi_networks.TablePlugin(client, logger),
		windows_associations.TablePlugin(client, logger),
		wmitable.TablePlugin(client, logger),
		dataflattentable.TablePluginExec(client, logger, "kolide_windows_updates",
			dataflattentable.JsonType,
			[]string{"powershell.exe", "-NoProfile", "-NonInteractive", wuUpdatesPowershell},
		),
		dataflattentable.TablePluginExec(client, logger, "kolide_windows_update_history",
			dataflattentable.JsonType,
			[]string{"powershell.exe", "-NoProfile", "-NonInteractive", wuHistoryPowershell},
		),
	}
}
