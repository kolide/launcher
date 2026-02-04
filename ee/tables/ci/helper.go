package ci

import "encoding/json"

// constraintExpr represents an expression in an osquery constraint
type constraintExpr struct {
	Op   int    `json:"op"`
	Expr string `json:"expr"`
}

// constraint represents an osquery column constraint
type constraint struct {
	Name string           `json:"name"`
	List []constraintExpr `json:"list"`
}

// constraintContext is the JSON structure for the osquery context
type constraintContext struct {
	Constraints []constraint `json:"constraints"`
}

// BuildRequestWithSingleEqualConstraint returns an osquery.ExtensionPluginRequest
// for a table generate call, adding a single constraint for the given column.
func BuildRequestWithSingleEqualConstraint(columnName string, columnValue string) map[string]string {
	ctx := constraintContext{
		Constraints: []constraint{
			{
				Name: columnName,
				List: []constraintExpr{
					{
						Op:   2,
						Expr: columnValue,
					},
				},
			},
		},
	}

	// Use json.Marshal to properly escape special characters (like backslashes in Windows paths)
	contextBytes, _ := json.Marshal(ctx)

	return map[string]string{
		"action":  "generate",
		"context": string(contextBytes),
	}
}
