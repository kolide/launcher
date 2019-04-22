package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/sshtable"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTablesCommon(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		BestPractices(client),
		sshtable.New(client, logger),
		ChromeLoginDataEmails(client, logger),
		ChromeUserProfiles(client, logger),
		EmailAddresses(client, logger),
		LauncherInfoTable(),
		OnePasswordAccounts(client, logger),
		SlackConfig(client, logger),
	}
}
