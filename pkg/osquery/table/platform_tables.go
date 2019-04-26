package table

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func platformTablesCommon(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	return []*table.Plugin{
		BestPractices(client),
		ChromeLoginDataEmails(client, logger),
		ChromeUserProfiles(client, logger),
		EmailAddresses(client, logger),
		KeyInfo(client, logger),
		LauncherInfoTable(),
		OnePasswordAccounts(client, logger),
		SlackConfig(client, logger),
		SshKeys(client, logger),
	}
}
