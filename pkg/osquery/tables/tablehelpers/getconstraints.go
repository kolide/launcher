package tablehelpers

import "github.com/kolide/osquery-go/plugin/table"

// GetConstraints returns a []string of the constraint expressions on
// a column. It's meant for the common, simple, usecase of iterating over them.
func GetConstraints(queryContext table.QueryContext, columnName string, defaults ...string) []string {
	q, ok := queryContext.Constraints[columnName]
	if !ok || len(q.Constraints) == 0 {
		return defaults
	}

	constraints := make([]string, len(q.Constraints))

	for i, c := range q.Constraints {
		constraints[i] = c.Expression
	}

	// FIXME: need uniq

	return constraints
}
