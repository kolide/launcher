// +build !darwin

package table

import "github.com/kolide/osquery-go/plugin/table"

func platformTables() []*table.Plugin {
	var tables []*table.Plugin
	return tables
}
