// package table provides custom osquery table implementations.
package table

import "github.com/kolide/osquery-go/plugin/table"

// PlatformTables returns all tables for the launcher build platform.
func PlatformTables() []*table.Plugin {
	return platformTables()
}
