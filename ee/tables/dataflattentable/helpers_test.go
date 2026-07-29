package dataflattentable

import (
	"testing"

	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func TestExtractPrefilterFromQuery(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName      string
		prefilterExprs    []string
		prefilterExpected bool
		errorExpected     bool
	}{
		{
			testCaseName: "valid prefilter",
			prefilterExprs: []string{`has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`},
			prefilterExpected: true,
			errorExpected:     false,
		},
		{
			testCaseName:      "invalid prefilter, error",
			prefilterExprs:    []string{`{`},
			prefilterExpected: false,
			errorExpected:     true,
		},
		{
			testCaseName: "multiple prefilters, error",
			prefilterExprs: []string{
				`has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`,
				`has(this.type) && this.type == "admin" ? {
  ?"timestamp": this.?timestamp
} : {}`,
			},
			prefilterExpected: false,
			errorExpected:     true,
		},
		{
			testCaseName:      "empty prefilter",
			prefilterExprs:    []string{},
			prefilterExpected: false,
			errorExpected:     false,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			qc := table.QueryContext{}

			if len(tt.prefilterExprs) > 0 {
				constraints := make([]table.Constraint, len(tt.prefilterExprs))
				for i, expr := range tt.prefilterExprs {
					constraints[i] = table.Constraint{Operator: table.OperatorEquals, Expression: expr}
				}
				qc.Constraints = map[string]table.ConstraintList{
					"prefilter": {
						Affinity:    "TEXT",
						Constraints: constraints,
					},
				}
			}

			p, err := ExtractPrefilterFromQuery(qc)

			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.prefilterExpected {
				require.NotNil(t, p)
			} else {
				require.Nil(t, p)
			}
		})
	}
}
