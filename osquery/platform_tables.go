// +build !darwin

package osquery

import "github.com/kolide/osquery-go/plugin/table"

func platformTables() []*table.Plugin {
	return []*table.Plugin{
		BestPractices(),
	}
}
