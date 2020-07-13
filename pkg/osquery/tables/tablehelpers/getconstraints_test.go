package tablehelpers

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetConstraints(t *testing.T) {
	t.Parallel()

	mockQC := MockQueryContext(map[string][]string{
		"empty_array":      []string{},
		"blank":            []string{""},
		"single":           []string{"a"},
		"double":           []string{"a", "b"},
		"duplicates":       []string{"a", "a", "b", "b"},
		"duplicate_blanks": []string{"a", "a", "", ""},
	})

	var tests = []struct {
		name     string
		expected []string
		defaults []string
	}{
		{
			name:     "does_not_exist",
			expected: []string(nil),
		},
		{
			name:     "does_not_exist_with_defaults",
			expected: []string{"a", "b"},
			defaults: []string{"a", "b"},
		},
		{
			name:     "empty_array",
			expected: []string{"a", "b"},
			defaults: []string{"a", "b"},
		},
		{
			name:     "empty_array",
			expected: []string(nil),
		},
		{
			name:     "blank",
			expected: []string{""},
		},
		{
			name:     "blank",
			expected: []string{""},
			defaults: []string{"a", "b"},
		},
		{
			name:     "single",
			expected: []string{"a"},
		},
		{
			name:     "single",
			expected: []string{"a"},
			defaults: []string{"a", "b"},
		},
		{
			name:     "double",
			expected: []string{"a", "b"},
		},
		{
			name:     "duplicates",
			expected: []string{"a", "b"},
		},
		{
			name:     "duplicate_blanks",
			expected: []string{"", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := GetConstraints(mockQC, tt.name, tt.defaults...)
			sort.Strings(actual)
			require.Equal(t, tt.expected, actual)
		})
	}

}
