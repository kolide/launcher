package ci

import "fmt"

// BuildRequestWithSingleEqualConstraint returns an osquery.ExtensionPluginRequest
// for a table generate call, adding a single constraint for the given column.
func BuildRequestWithSingleEqualConstraint(columnName string, columnValue string) map[string]string {
	return map[string]string{
		"action": "generate",
		"context": fmt.Sprintf(`{
"constraints": [
	{
		"name": "%s",
		"list": [
			{
				"op": 2,
				"expr": "%s"
			}
		]
	}
]
}`, columnName, columnValue),
	}
}
