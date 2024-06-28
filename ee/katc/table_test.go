package katc

import (
	"path/filepath"
	"testing"

	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_sourcePathAdheresToSourceConstraints(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName  string
		sourcePath    string
		constraints   table.ConstraintList
		adheres       bool
		errorExpected bool
	}{
		{
			testCaseName: "equals",
			sourcePath:   filepath.Join("some", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("some", "path", "to", "a", "source"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "not equals",
			sourcePath:   filepath.Join("some", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("a", "path", "to", "a", "different", "source"),
					},
				},
			},
			adheres:       false,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE with % wildcard",
			sourcePath:   filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("a", "path", "to", "%.sqlite"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE with underscore wildcard",
			sourcePath:   filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("_", "path", "to", "db.sqlite"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE is case-insensitive",
			sourcePath:   filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("A", "PATH", "TO", "DB.%"),
					},
				},
			},
			adheres: true,
		},
		{
			testCaseName: "GLOB with * wildcard",
			sourcePath:   filepath.Join("another", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorGlob,
						Expression: filepath.Join("another", "*", "to", "a", "source"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "GLOB with ? wildcard",
			sourcePath:   filepath.Join("another", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorGlob,
						Expression: filepath.Join("another", "path", "to", "?", "source"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "regexp",
			sourcePath:   filepath.Join("test", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorRegexp,
						Expression: `.*source$`,
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
		{
			testCaseName: "invalid regexp",
			sourcePath:   filepath.Join("test", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorRegexp,
						Expression: `invalid\`,
					},
				},
			},
			adheres:       false,
			errorExpected: true,
		},
		{
			testCaseName: "unsupported",
			sourcePath:   filepath.Join("test", "path", "to", "a", "source", "2"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorUnique,
						Expression: filepath.Join("test", "path", "to", "a", "source", "2"),
					},
				},
			},
			adheres:       false,
			errorExpected: true,
		},
		{
			testCaseName: "multiple constraints where one does not match",
			sourcePath:   filepath.Join("test", "path", "to", "a", "source", "3"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("test", "path", "to", "a", "source", "%"),
					},
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("some", "path", "to", "a", "source"),
					},
				},
			},
			adheres:       false,
			errorExpected: false,
		},
		{
			testCaseName: "multiple constraints where all match",
			sourcePath:   filepath.Join("test", "path", "to", "a", "source", "3"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("test", "path", "to", "a", "source", "%"),
					},
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("test", "path", "to", "a", "source", "3"),
					},
				},
			},
			adheres:       true,
			errorExpected: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			adheres, err := sourcePathAdheresToSourceConstraints(tt.sourcePath, &tt.constraints)
			if tt.errorExpected {
				require.Error(t, err, "expected error on checking constraints")
			} else {
				require.NoError(t, err, "expected no error on checking constraints")
			}

			require.Equal(t, tt.adheres, adheres, "incorrect result checking constraints")
		})
	}
}
