package tablehelpers

import (
	"github.com/kolide/osquery-go/plugin/table"
)

// GetConstraints returns a []string of the constraint expressions on
// a column. It's meant for the common, simple, usecase of iterating over them.
func GetConstraints(queryContext table.QueryContext, columnName string, defaults ...string) []string {
	q, ok := queryContext.Constraints[columnName]
	if !ok || len(q.Constraints) == 0 {
		return defaults
	}

	constraintSet := make(map[string]struct{})

	for _, c := range q.Constraints {
		constraintSet[c.Expression] = struct{}{}
	}

	constraints := make([]string, len(constraintSet))

	i := 0
	for key := range constraintSet {
		constraints[i] = key
		i++
	}

	return constraints
}
