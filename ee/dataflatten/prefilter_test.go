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

func TestApply(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName   string
		prefilterExpr  string
		dataToFilter   any
		expectedResult any
	}{
		{
			testCaseName: "prefilter matches, strips unnecessary fields",
			prefilterExpr: `has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`,
			dataToFilter: map[string]any{
				"type":       "user",
				"some-field": "abc",
				"timestamp":  "12345", // string just to avoid annoying integer type comparisons
			},
			expectedResult: map[string]any{
				"timestamp": "12345",
			},
		},
		{
			testCaseName: "prefilter does not match",
			prefilterExpr: `has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`,
			dataToFilter: map[string]any{
				"type":       "admin",
				"some-field": "abc",
				"timestamp":  "12345",
			},
			expectedResult: nil,
		},
		{
			testCaseName: "prefilter field is missing, handled gracefully",
			prefilterExpr: `has(this.type) && this.type == "user" ? {
  ?"timestamp": this.?timestamp
} : {}`,
			dataToFilter: map[string]any{
				"some-field": "abc",
				"timestamp":  "12345",
			},
			expectedResult: nil,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			p, err := NewPrefilter(tt.prefilterExpr)
			require.NoError(t, err)

			result, err := p.Apply(tt.dataToFilter)
			require.NoError(t, err)
			require.Equal(t, tt.expectedResult, result)
		})
	}
}
