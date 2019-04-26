package table

import (
	"github.com/kolide/launcher/pkg/launcher"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

// LauncherTables returns launcher-specific tables
func LauncherTables(db *bolt.DB, opts *launcher.Options) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(db),
		LauncherIdentifierTable(db),
		TargetMembershipTable(db),
		LauncherAutoupdateConfigTable(opts),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	// Common tables to all platforms
	tables := []*table.Plugin{
		BestPractices(client),
		ChromeLoginDataEmails(client, logger),
		ChromeUserProfiles(client, logger),
		EmailAddresses(client, logger),
		LauncherInfoTable(),
		OnePasswordAccounts(client, logger),
		SlackConfig(client, logger),
	}

	// add in the platform specific ones (as denboted by build tags)
	tables = append(tables, platformTables(client, logger)...)

	return tables
}
