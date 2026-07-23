package dataflatten

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPrefilter(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName  string
		prefilterExpr string
		errorExpected bool
	}{
		{
			testCaseName: "valid prefilter",
			prefilterExpr: `has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`,
			errorExpected: false,
		},
		{
			testCaseName: "valid prefilter with invalid variable",
			prefilterExpr: `record.type == "user" ? {
  ?"timestamp":              record.?timestamp
} : {}`,
			errorExpected: true,
		},
		{
			testCaseName:  "invalid prefilter, parse error",
			prefilterExpr: `{`,
			errorExpected: true,
		},
		{
			testCaseName:  "invalid prefilter, compile error",
			prefilterExpr: `1 + "a"`,
			errorExpected: true,
		},
		{
			testCaseName:  "empty prefilter",
			prefilterExpr: "",
			errorExpected: true,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			p, err := NewPrefilter(tt.prefilterExpr)
			if tt.errorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, p)
				// Evaluate an empty set of data as a quick smoke test
				_, err = p.Apply(map[string]any{})
				require.NoError(t, err)
			}
		})
	}
}
