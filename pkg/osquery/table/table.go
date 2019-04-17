package table

import (
	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

// LauncherTables returns launcher-specific tables
func LauncherTables(db *bolt.DB) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		LauncherConfigTable(db),
		LauncherIdentifierTable(db),
		TargetMembershipTable(db),
	}
}

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables(client *osquery.ExtensionManagerClient, logger log.Logger) []*table.Plugin {
	tables := platformTables(client, logger)
	tables = append(tables, platformTablesCommon(client, logger)...)
	return tables
}
